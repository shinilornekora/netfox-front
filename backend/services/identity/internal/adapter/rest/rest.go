// Package rest — HTTP API identity.
package rest

import (
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"plenka/identity/internal/core"
	"plenka/identity/internal/usecase"
	"plenka/kit/principal"
	"plenka/kit/problem"
	"plenka/kit/web"
)

const Cookie = "plenka_sid"

type API struct {
	UC           *usecase.Service
	SecureCookie bool
}

type accountDTO struct {
	ID          string    `json:"id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name"`
	BirthDate   *string   `json:"birth_date"`
	Role        string    `json:"role"`
	CreatedAt   time.Time `json:"created_at"`
	Revision    int64     `json:"revision"`
}

func dto(a core.Account) accountDTO {
	d := accountDTO{ID: a.ID.String(), Email: a.Email, DisplayName: a.DisplayName, Role: string(a.Role), CreatedAt: a.CreatedAt, Revision: a.Revision}
	if a.BirthDate != nil {
		s := a.BirthDate.Format("2006-01-02")
		d.BirthDate = &s
	}
	return d
}

func (api *API) Mount(r chi.Router, h func(web.Handler) http.HandlerFunc) {
	r.Post("/api/auth/register", h(api.register))
	r.Post("/api/auth/login", h(api.login))
	r.Post("/api/auth/logout", h(api.logout))
	r.Get("/api/auth/session", h(api.session))
	r.Get("/api/me", h(api.me))
	r.Patch("/api/me", h(api.updateMe))
	r.Post("/api/me/password", h(api.password))
	r.Get("/api/me/devices", h(api.devices))
	r.Delete("/api/me/devices/{id}", h(api.revoke))
}

func device(r *http.Request) usecase.Device {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	// Gateway ставит X-Forwarded-For = адрес клиента, а httputil.ReverseProxy
	// дописывает к нему адрес самого gateway. Клиент — первый элемент.
	if f := r.Header.Get("X-Forwarded-For"); f != "" {
		ip = strings.TrimSpace(strings.Split(f, ",")[0])
	}
	return usecase.Device{UserAgent: r.UserAgent(), IP: ip}
}

// Cookie сессии: HttpOnly — недоступна JS; SameSite=Lax — не уходит с
// межсайтовыми POST (основа защиты от CSRF вместе с заголовком, который
// проверяет gateway); Path=/ — нужна всем маршрутам API и потоку видео.
func (api *API) setCookie(w http.ResponseWriter, l usecase.Login) {
	http.SetCookie(w, &http.Cookie{Name: Cookie, Value: l.Token, Path: "/", Expires: l.Expires,
		HttpOnly: true, Secure: api.SecureCookie, SameSite: http.SameSiteLaxMode})
}

func (api *API) clearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: Cookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true,
		Secure: api.SecureCookie, SameSite: http.SameSiteLaxMode})
}

func token(r *http.Request) string {
	if c, err := r.Cookie(Cookie); err == nil {
		return c.Value
	}
	return ""
}

func (api *API) register(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Email       string `json:"email"`
		Password    string `json:"password"`
		DisplayName string `json:"display_name"`
		BirthDate   string `json:"birth_date"`
	}
	if err := web.Bind(r, &in); err != nil {
		return err
	}
	l, err := api.UC.Register(r.Context(), core.Draft{Email: in.Email, Password: in.Password, DisplayName: in.DisplayName, BirthDate: in.BirthDate}, device(r))
	if err != nil {
		return err
	}
	api.setCookie(w, l)
	return web.JSON(w, http.StatusCreated, dto(l.Account))
}

func (api *API) login(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := web.Bind(r, &in); err != nil {
		return err
	}
	l, err := api.UC.Login(r.Context(), in.Email, in.Password, device(r))
	if err != nil {
		return err
	}
	api.setCookie(w, l)
	return web.JSON(w, http.StatusOK, dto(l.Account))
}

func (api *API) logout(w http.ResponseWriter, r *http.Request) error {
	if err := api.UC.Logout(r.Context(), token(r)); err != nil {
		return err
	}
	api.clearCookie(w)
	return web.NoContent(w)
}

func (api *API) me(w http.ResponseWriter, r *http.Request) error {
	a, err := api.UC.Me(r.Context())
	if err != nil {
		return err
	}
	return web.JSON(w, 200, dto(a))
}

// session — «кто я» для SPA при загрузке. В отличие от /api/me, гостю
// отвечает 200 {"user": null}: отсутствие входа — нормальное состояние,
// а не ошибка, и браузер не сыплет 401 в консоль на каждой загрузке.
func (api *API) session(w http.ResponseWriter, r *http.Request) error {
	if _, ok := principal.From(r.Context()); !ok {
		return web.JSON(w, 200, map[string]any{"user": nil})
	}
	a, err := api.UC.Me(r.Context())
	if problem.Is(err, "unauthenticated") || problem.Is(err, "not_found") {
		return web.JSON(w, 200, map[string]any{"user": nil})
	}
	if err != nil {
		return err
	}
	return web.JSON(w, 200, map[string]any{"user": dto(a)})
}

func (api *API) updateMe(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		DisplayName string `json:"display_name"`
		BirthDate   string `json:"birth_date"`
		Revision    int64  `json:"revision"`
	}
	if err := web.Bind(r, &in); err != nil {
		return err
	}
	a, err := api.UC.UpdateProfile(r.Context(), in.DisplayName, in.BirthDate, in.Revision)
	if err != nil {
		return err
	}
	return web.JSON(w, 200, dto(a))
}

func (api *API) currentSession(r *http.Request) uuid.UUID {
	_, s, ok, _ := api.UC.Resolve(r.Context(), token(r))
	if !ok {
		return uuid.Nil
	}
	return s.ID
}

func (api *API) password(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Current string `json:"current_password"`
		New     string `json:"new_password"`
	}
	if err := web.Bind(r, &in); err != nil {
		return err
	}
	if err := api.UC.ChangePassword(r.Context(), api.currentSession(r), in.Current, in.New); err != nil {
		return err
	}
	return web.NoContent(w)
}

type deviceDTO struct {
	ID         string    `json:"id"`
	UserAgent  string    `json:"user_agent"`
	IP         string    `json:"ip"`
	CreatedAt  time.Time `json:"created_at"`
	LastSeenAt time.Time `json:"last_seen_at"`
	Current    bool      `json:"current"`
}

func (api *API) devices(w http.ResponseWriter, r *http.Request) error {
	list, err := api.UC.Devices(r.Context())
	if err != nil {
		return err
	}
	cur := api.currentSession(r)
	out := make([]deviceDTO, 0, len(list))
	for _, s := range list {
		out = append(out, deviceDTO{ID: s.ID.String(), UserAgent: s.UserAgent, IP: s.IP, CreatedAt: s.CreatedAt, LastSeenAt: s.LastSeenAt, Current: s.ID == cur})
	}
	return web.JSON(w, 200, map[string]any{"items": out})
}

func (api *API) revoke(w http.ResponseWriter, r *http.Request) error {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		return problem.NotFound("Сессия")
	}
	if _, err := principal.Need(r.Context()); err != nil {
		return err
	}
	if err := api.UC.RevokeDevice(r.Context(), id); err != nil {
		return err
	}
	return web.NoContent(w)
}
