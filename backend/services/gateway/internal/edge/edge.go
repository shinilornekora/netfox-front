// Package edge — входной шлюз Плёнки.
//
// Порядок обработки запроса /api/*:
//  1. CSRF: изменяющие запросы обязаны нести заголовок X-Plenka-Client —
//     межсайтовая HTML-форма его поставить не может, а межсайтовый fetch
//     с ним упрётся в CORS (мы его не разрешаем).
//  2. Заголовки X-Plenka-* от клиента вырезаются.
//  3. Cookie сессии превращается в пользователя (identity по gRPC, кэш).
//  4. Лимит частоты — скользящее окно по пользователю или IP.
//  5. Bulkhead: у каждого сервиса свой лимит одновременных запросов.
//  6. Подписанные заголовки личности → проксирование.
package edge

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

	identityv1 "plenka/contracts/gen/identity/v1"
	"plenka/kit/principal"
	"plenka/kit/problem"
)

const SessionCookie = "plenka_sid"

// ---------- сессии ----------

type cached struct {
	p     *principal.Principal // nil — сессия недействительна
	until time.Time
}

// Sessions разрешает токен через identity и кэширует ответ. Положительный
// ответ живёт PosTTL, отрицательный — NegTTL (защита identity от перебора
// мусорных cookie). Цена кэша: отозванная сессия ещё до PosTTL видна этому
// экземпляру gateway, если отзыв прошёл не через него.
type Sessions struct {
	RPC    identityv1.SessionsClient
	PosTTL time.Duration
	NegTTL time.Duration
	Budget time.Duration

	mu    sync.Mutex
	cache map[string]cached
}

func (s *Sessions) Resolve(ctx context.Context, token string) (*principal.Principal, error) {
	now := time.Now()
	s.mu.Lock()
	if s.cache == nil {
		s.cache = map[string]cached{}
	}
	c, ok := s.cache[token]
	s.mu.Unlock()
	if ok && now.Before(c.until) {
		return c.p, nil
	}
	cctx, cancel := context.WithTimeout(ctx, s.Budget)
	defer cancel()
	resp, err := s.RPC.Resolve(cctx, &identityv1.ResolveRequest{Token: token})
	if err != nil {
		return nil, err
	}
	entry := cached{until: now.Add(s.NegTTL)}
	if resp.GetValid() {
		entry = cached{p: &principal.Principal{UserID: resp.GetUserId(), Role: principal.Role(resp.GetRole()),
			BirthYear: int(resp.GetBirthYear())}, until: now.Add(s.PosTTL)}
	}
	s.mu.Lock()
	if len(s.cache) > 100_000 {
		s.cache = map[string]cached{}
	}
	s.cache[token] = entry
	s.mu.Unlock()
	return entry.p, nil
}

// Forget выкидывает токен из кэша (выход из аккаунта через этот gateway).
func (s *Sessions) Forget(token string) {
	s.mu.Lock()
	delete(s.cache, token)
	s.mu.Unlock()
}

// ---------- лимит частоты: скользящее окно ----------

// Window считает запросы в двух соседних фиксированных окнах и взвешивает
// предыдущее по доле, ещё попадающей в скользящее окно. В отличие от
// «фиксированного окна», не пропускает двойной всплеск на стыке минут,
// и в отличие от журнала меток времени, хранит два числа на ключ.
type Window struct {
	Limit int
	Size  time.Duration

	mu sync.Mutex
	m  map[string]*win
}

type win struct {
	start      time.Time
	prev, curr int
}

func (w *Window) Allow(key string, now time.Time) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.m == nil {
		w.m = map[string]*win{}
	}
	cur := now.Truncate(w.Size)
	x, ok := w.m[key]
	switch {
	case !ok:
		x = &win{start: cur}
		w.m[key] = x
	case cur.Sub(x.start) >= 2*w.Size:
		x.start, x.prev, x.curr = cur, 0, 0
	case cur.Sub(x.start) >= w.Size:
		x.start, x.prev, x.curr = cur, x.curr, 0
	}
	weight := 1 - float64(now.Sub(cur))/float64(w.Size)
	if float64(x.prev)*weight+float64(x.curr) >= float64(w.Limit) {
		return false
	}
	x.curr++
	if len(w.m) > 200_000 {
		w.m = map[string]*win{key: x}
	}
	return true
}

// ---------- bulkhead ----------

// Upstream — сервис за шлюзом. Slots — сколько запросов к нему может быть
// в полёте одновременно. Если все слоты заняты, запрос сразу получает 503,
// а не встаёт в очередь: зависший billing не съест горутины и соединения,
// нужные каталогу (изоляция отсеков, как в корабле).
type Upstream struct {
	Name     string
	URL      *url.URL
	Prefixes []string
	Timeout  time.Duration // 0 — без общего тайм-аута (поток видео)
	Slots    int

	sem   chan struct{}
	proxy *httputil.ReverseProxy
}

type Edge struct {
	Log       *slog.Logger
	Signer    principal.Signer
	Sessions  *Sessions
	Upstreams []*Upstream
	PerUser   *Window
	AuthLimit *Window
	TrustXFF  bool
}

func (e *Edge) Init() {
	tr := &http.Transport{
		DialContext:           (&net.Dialer{Timeout: 2 * time.Second}).DialContext,
		MaxIdleConnsPerHost:   64,
		IdleConnTimeout:       90 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
	}
	for _, u := range e.Upstreams {
		u := u
		u.sem = make(chan struct{}, u.Slots)
		p := httputil.NewSingleHostReverseProxy(u.URL)
		p.Transport = tr
		p.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
			if errors.Is(err, context.Canceled) {
				return
			}
			e.Log.Warn("upstream error", "upstream", u.Name, "err", err)
			code := "upstream_unavailable"
			if errors.Is(err, context.DeadlineExceeded) {
				code = "upstream_timeout"
			}
			problem.Write(w, problem.Unavailable(code, "Сервис временно недоступен, попробуйте ещё раз"))
		}
		u.proxy = p
	}
}

func (e *Edge) route(path string) *Upstream {
	var best *Upstream
	n := -1
	for _, u := range e.Upstreams {
		for _, p := range u.Prefixes {
			if (path == p || strings.HasPrefix(path, p+"/")) && len(p) > n {
				best, n = u, len(p)
			}
		}
	}
	return best
}

func (e *Edge) ip(r *http.Request) string {
	if e.TrustXFF {
		if f := r.Header.Get("X-Forwarded-For"); f != "" {
			return strings.TrimSpace(strings.Split(f, ",")[0])
		}
	}
	h, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return h
}

func unsafe(m string) bool {
	return m != http.MethodGet && m != http.MethodHead && m != http.MethodOptions
}

func (e *Edge) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "same-origin")

	if unsafe(r.Method) && r.Header.Get("X-Plenka-Client") == "" {
		problem.Write(w, problem.New(403, "csrf", "Запрос отклонён: нет заголовка клиента"))
		return
	}
	principal.Strip(r.Header)

	up := e.route(r.URL.Path)
	if up == nil {
		problem.Write(w, problem.NotFound("Маршрут"))
		return
	}

	ip := e.ip(r)
	var who *principal.Principal
	if c, err := r.Cookie(SessionCookie); err == nil && c.Value != "" {
		p, err := e.Sessions.Resolve(r.Context(), c.Value)
		if err != nil {
			// identity недоступен: не пускать анонимом под чужой cookie.
			e.Log.Warn("session resolve failed", "err", err)
			problem.Write(w, problem.Unavailable("auth_unavailable", "Не удалось проверить вход, попробуйте через минуту"))
			return
		}
		who = p
		if r.URL.Path == "/api/auth/logout" {
			e.Sessions.Forget(c.Value)
		}
	}

	key := "ip:" + ip
	if who != nil {
		key = "u:" + who.UserID
	}
	now := time.Now()
	// Лимит на вход/регистрацию — против перебора паролей. Выход не
	// ограничиваем: запрет выйти из аккаунта хуже любого перебора.
	if strings.HasPrefix(r.URL.Path, "/api/auth/") && r.URL.Path != "/api/auth/logout" && unsafe(r.Method) &&
		!e.AuthLimit.Allow("ip:"+ip, now) {
		tooMany(w)
		return
	}
	if !strings.HasPrefix(r.URL.Path, "/api/playback/stream/") && !e.PerUser.Allow(key, now) {
		tooMany(w)
		return
	}

	select {
	case up.sem <- struct{}{}:
		defer func() { <-up.sem }()
	default:
		h.Set("Retry-After", "2")
		problem.Write(w, problem.Unavailable("busy", "Сервис перегружен, повторите через пару секунд"))
		return
	}

	if who != nil {
		e.Signer.Stamp(r, *who)
	}
	r.Header.Set("X-Forwarded-For", ip)
	if up.Timeout > 0 {
		ctx, cancel := context.WithTimeout(r.Context(), up.Timeout)
		defer cancel()
		r = r.WithContext(ctx)
	}
	up.proxy.ServeHTTP(w, r)
}

func tooMany(w http.ResponseWriter) {
	w.Header().Set("Retry-After", "10")
	problem.Write(w, problem.New(429, "rate_limited", "Слишком много запросов — подождите немного"))
}
