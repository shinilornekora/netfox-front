package edge

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"

	identityv1 "plenka/contracts/gen/identity/v1"
	"plenka/kit/principal"
)

type fakeSessions struct {
	calls atomic.Int32
	down  bool
}

func (f *fakeSessions) Resolve(_ context.Context, in *identityv1.ResolveRequest, _ ...grpc.CallOption) (*identityv1.ResolveResponse, error) {
	f.calls.Add(1)
	if f.down {
		return nil, errors.New("identity down")
	}
	if in.GetToken() == "good" {
		return &identityv1.ResolveResponse{Valid: true, UserId: "u1", Role: "viewer", BirthYear: 1999}, nil
	}
	return &identityv1.ResolveResponse{Valid: false}, nil
}

var key = []byte("internal")

func build(t *testing.T, h http.Handler, slots int) (*Edge, *fakeSessions) {
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	fs := &fakeSessions{}
	e := &Edge{
		Log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		Signer:    principal.Signer{Key: key},
		Sessions:  &Sessions{RPC: fs, PosTTL: time.Minute, NegTTL: time.Minute, Budget: time.Second},
		Upstreams: []*Upstream{{Name: "up", URL: u, Prefixes: []string{"/api/titles", "/api/auth"}, Slots: slots}},
		PerUser:   &Window{Limit: 1000, Size: time.Minute},
		AuthLimit: &Window{Limit: 3, Size: time.Minute},
	}
	e.Init()
	return e, fs
}

func do(e *Edge, method, path, cookie string, hdr map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, nil)
	if cookie != "" {
		r.AddCookie(&http.Cookie{Name: SessionCookie, Value: cookie})
	}
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	e.ServeHTTP(w, r)
	return w
}

func TestCSRFAndIdentityHeaders(t *testing.T) {
	var seen http.Header
	var seenReq *http.Request
	e, fs := build(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Clone()
		seenReq = r.Clone(r.Context())
	}), 4)

	if w := do(e, "POST", "/api/titles", "good", nil); w.Code != 403 {
		t.Fatalf("POST without X-Plenka-Client: %d", w.Code)
	}
	// Клиент пытается выдать себя за админа — заголовки вырезаются и
	// заменяются подписанными настоящими.
	forged := map[string]string{principal.HUser: "admin-id", principal.HRole: "admin", principal.HSig: "x"}
	if w := do(e, "GET", "/api/titles", "good", forged); w.Code != 200 {
		t.Fatalf("GET: %d", w.Code)
	}
	p, ok, err := principal.Signer{Key: key}.Read(seenReq)
	if !ok || err != nil || p.UserID != "u1" || p.Role != principal.Viewer {
		t.Fatalf("upstream principal: %+v %v %v", p, ok, err)
	}
	// Те же подписанные заголовки, приложенные к другому запросу, не принимаются.
	replay := httptest.NewRequest("DELETE", "/api/admin/titles/x", nil)
	replay.Header = seen.Clone()
	if _, _, err := (principal.Signer{Key: key}).Read(replay); err == nil {
		t.Fatal("signature replayed on another method/path")
	}
	// Анонимный запрос с поддельными заголовками приходит без них.
	do(e, "GET", "/api/titles", "", forged)
	if seen.Get(principal.HUser) != "" || seen.Get(principal.HSig) != "" {
		t.Fatal("forged headers leaked to upstream")
	}
	// Повторные запросы — из кэша, без gRPC.
	before := fs.calls.Load()
	for i := 0; i < 5; i++ {
		do(e, "GET", "/api/titles", "good", nil)
	}
	if fs.calls.Load() != before {
		t.Fatal("session cache not used")
	}
	// Выход сбрасывает кэш.
	do(e, "POST", "/api/auth/logout", "good", map[string]string{"X-Plenka-Client": "web"})
	do(e, "GET", "/api/titles", "good", nil)
	if fs.calls.Load() != before+1 {
		t.Fatalf("logout must forget the session: calls %d", fs.calls.Load()-before)
	}
	if w := do(e, "GET", "/api/nope", "", nil); w.Code != 404 {
		t.Fatalf("unknown route: %d", w.Code)
	}
}

func TestIdentityDownIsNotAnonymous(t *testing.T) {
	e, fs := build(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), 4)
	fs.down = true
	if w := do(e, "GET", "/api/titles", "good", nil); w.Code != 503 {
		t.Fatalf("identity down with a cookie must be 503, got %d", w.Code)
	}
	if w := do(e, "GET", "/api/titles", "", nil); w.Code != 200 {
		t.Fatalf("anonymous still works: %d", w.Code)
	}
}

func TestAuthRateLimit(t *testing.T) {
	e, _ := build(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), 4)
	h := map[string]string{"X-Plenka-Client": "web"}
	for i := 0; i < 3; i++ {
		if w := do(e, "POST", "/api/auth/login", "", h); w.Code != 200 {
			t.Fatalf("attempt %d: %d", i, w.Code)
		}
	}
	w := do(e, "POST", "/api/auth/login", "", h)
	if w.Code != 429 || w.Header().Get("Retry-After") == "" {
		t.Fatalf("4th login: %d", w.Code)
	}
	if w := do(e, "POST", "/api/auth/logout", "", h); w.Code != 200 {
		t.Fatalf("logout is never rate limited: %d", w.Code)
	}
}

func TestBulkhead(t *testing.T) {
	release := make(chan struct{})
	var entered sync.WaitGroup
	entered.Add(2)
	var held atomic.Int32
	e, _ := build(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		if held.Add(1) <= 2 {
			entered.Done()
			<-release
		}
	}), 2)
	var done sync.WaitGroup
	for i := 0; i < 2; i++ {
		done.Add(1)
		go func() { defer done.Done(); do(e, "GET", "/api/titles", "", nil) }()
	}
	entered.Wait()
	if w := do(e, "GET", "/api/titles", "", nil); w.Code != 503 {
		t.Fatalf("third concurrent request must be shed: %d", w.Code)
	}
	close(release)
	done.Wait()
	if w := do(e, "GET", "/api/titles", "", nil); w.Code != 200 {
		t.Fatalf("after release: %d", w.Code)
	}
}

func TestSlidingWindowNoDoubleBurst(t *testing.T) {
	w := &Window{Limit: 10, Size: time.Minute}
	t0 := time.Unix(600, 0) // начало окна
	n := 0
	for i := 0; i < 10; i++ {
		if w.Allow("k", t0.Add(59*time.Second)) {
			n++
		}
	}
	// Сразу после границы фиксированное окно пропустило бы ещё 10.
	for i := 0; i < 10; i++ {
		if w.Allow("k", t0.Add(61*time.Second)) {
			n++
		}
	}
	if n > 11 {
		t.Fatalf("double burst on window boundary: %d", n)
	}
	if !w.Allow("k", t0.Add(3*time.Minute)) {
		t.Fatal("window must recover")
	}
}
