// Package principal — кто выполняет запрос.
//
// Аутентификацию делает gateway: по cookie сессии он узнаёт пользователя у
// identity и передаёт его сервисам заголовками X-Plenka-*. Чтобы сервис не
// поверил заголовкам, подставленным в обход gateway, заголовки подписаны
// HMAC общим внутренним ключом и действительны 60 секунд. В подпись входят
// метод и путь запроса: перехваченные заголовки нельзя приложить к другому
// запросу (например, подпись от GET /api/titles — к DELETE /api/admin/...).
package principal

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"time"

	"plenka/kit/problem"
)

type Role string

const (
	Viewer Role = "viewer"
	Admin  Role = "admin"
)

type Principal struct {
	UserID    string
	Role      Role
	BirthYear int // 0 — не указан
}

// Age — полных лет на текущий год (приближённо, по году рождения).
func (p Principal) Age(now time.Time) int {
	if p.BirthYear == 0 {
		return 0
	}
	return now.Year() - p.BirthYear
}

const (
	HUser  = "X-Plenka-User"
	HRole  = "X-Plenka-Role"
	HBirth = "X-Plenka-Birth"
	HTime  = "X-Plenka-Ts"
	HSig   = "X-Plenka-Sig"
)

// MaxSkew — сколько живёт подпись.
const MaxSkew = 60 * time.Second

type Signer struct {
	Key []byte
	Now func() time.Time
}

func (s Signer) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s Signer) mac(method, path, user, role, birth, ts string) string {
	m := hmac.New(sha256.New, s.Key)
	m.Write([]byte(method + "\n" + path + "\n" + user + "\n" + role + "\n" + birth + "\n" + ts))
	return hex.EncodeToString(m.Sum(nil))
}

// Stamp кладёт подписанные заголовки в запрос (gateway).
func (s Signer) Stamp(r *http.Request, p Principal) {
	h := r.Header
	ts := strconv.FormatInt(s.now().Unix(), 10)
	birth := strconv.Itoa(p.BirthYear)
	h.Set(HUser, p.UserID)
	h.Set(HRole, string(p.Role))
	h.Set(HBirth, birth)
	h.Set(HTime, ts)
	h.Set(HSig, s.mac(r.Method, r.URL.EscapedPath(), p.UserID, string(p.Role), birth, ts))
}

// Strip удаляет заголовки, пришедшие от клиента (gateway, до Stamp).
func Strip(h http.Header) {
	for _, k := range []string{HUser, HRole, HBirth, HTime, HSig} {
		h.Del(k)
	}
}

// Read проверяет подпись. ok=false — анонимный запрос; err — подделка.
func (s Signer) Read(r *http.Request) (Principal, bool, error) {
	h := r.Header
	user, sig := h.Get(HUser), h.Get(HSig)
	if user == "" && sig == "" {
		return Principal{}, false, nil
	}
	role, birth, ts := h.Get(HRole), h.Get(HBirth), h.Get(HTime)
	if !hmac.Equal([]byte(sig), []byte(s.mac(r.Method, r.URL.EscapedPath(), user, role, birth, ts))) {
		return Principal{}, false, problem.New(http.StatusUnauthorized, "bad_signature", "Запрос не прошёл через gateway")
	}
	sec, err := strconv.ParseInt(ts, 10, 64)
	if err != nil || s.now().Sub(time.Unix(sec, 0)).Abs() > MaxSkew {
		return Principal{}, false, problem.New(http.StatusUnauthorized, "stale_signature", "Подпись запроса устарела")
	}
	b, _ := strconv.Atoi(birth)
	rl := Role(role)
	if rl != Viewer && rl != Admin {
		return Principal{}, false, problem.New(http.StatusUnauthorized, "bad_role", "Неизвестная роль")
	}
	return Principal{UserID: user, Role: rl, BirthYear: b}, true, nil
}

type key struct{}

func With(ctx context.Context, p Principal) context.Context { return context.WithValue(ctx, key{}, p) }

func From(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(key{}).(Principal)
	return p, ok
}

// Need — пользователь обязателен; roles — допустимые роли (пусто — любая).
func Need(ctx context.Context, roles ...Role) (Principal, error) {
	p, ok := From(ctx)
	if !ok {
		return p, problem.Unauthenticated()
	}
	if len(roles) == 0 {
		return p, nil
	}
	for _, r := range roles {
		if p.Role == r {
			return p, nil
		}
	}
	return p, problem.Forbidden("Раздел доступен только " + roleNames(roles))
}

func roleNames(roles []Role) string {
	names := make([]string, 0, len(roles))
	for _, r := range roles {
		switch r {
		case Admin:
			names = append(names, "администратору")
		case Viewer:
			names = append(names, "зрителю")
		}
	}
	return strings.Join(names, " и ")
}

// Middleware читает подписанные заголовки и кладёт пользователя в контекст.
func (s Signer) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok, err := s.Read(r)
		if err != nil {
			problem.Write(w, problem.From(err))
			return
		}
		if ok {
			r = r.WithContext(With(r.Context(), p))
		}
		next.ServeHTTP(w, r)
	})
}
