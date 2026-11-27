// Package rest — HTTP API playback.
package rest

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"plenka/kit/problem"
	"plenka/kit/web"
	"plenka/playback/internal/core"
	"plenka/playback/internal/usecase"
)

type API struct {
	UC       *usecase.Service
	MediaDir string
}

func (a *API) Mount(r chi.Router, h func(web.Handler) http.HandlerFunc) {
	r.Post("/api/playback/start", h(a.start))
	r.Get("/api/playback/stream/{token}", h(a.stream))
	r.Put("/api/playback/progress", h(a.progress))
	r.Get("/api/playback/titles/{id}", h(a.titleState))
	r.Get("/api/playback/continue", h(a.cont))
	r.Get("/api/playback/history", h(a.history))
	r.Get("/api/playback/watchlist", h(a.watchlist))
	r.Put("/api/playback/watchlist/{id}", h(a.addWatch))
	r.Delete("/api/playback/watchlist/{id}", h(a.removeWatch))
}

type titleRef struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Year      int    `json:"year"`
	AgeRating int    `json:"age_rating"`
	PosterHue int    `json:"poster_hue"`
	PosterURL string `json:"poster_url,omitempty"`
	Available bool   `json:"available"`
}

func ref(t core.Title, ok bool, id uuid.UUID) titleRef {
	if !ok {
		return titleRef{ID: id.String(), Name: "Недоступно", Available: false}
	}
	r := titleRef{ID: t.ID.String(), Name: t.Name, Kind: t.Kind, Year: t.Year, AgeRating: t.AgeRating, PosterHue: t.PosterHue, Available: t.Status == "published"}
	if t.PosterRev > 0 {
		r.PosterURL = fmt.Sprintf("/api/titles/%s/poster?v=%d", t.ID, t.PosterRev)
	}
	return r
}

func pathID(r *http.Request) (uuid.UUID, error) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		return uuid.Nil, problem.NotFound("Тайтл")
	}
	return id, nil
}

func optUUID(s string) (*uuid.UUID, error) {
	if s == "" {
		return nil, nil
	}
	id, err := uuid.Parse(s)
	if err != nil {
		return nil, problem.NotFound("Серия")
	}
	return &id, nil
}

func (a *API) start(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		TitleID   string `json:"title_id"`
		EpisodeID string `json:"episode_id"`
	}
	if err := web.Bind(r, &in); err != nil {
		return err
	}
	tid, err := uuid.Parse(in.TitleID)
	if err != nil {
		return problem.NotFound("Тайтл")
	}
	ep, err := optUUID(in.EpisodeID)
	if err != nil {
		return err
	}
	st, err := a.UC.Play(r.Context(), tid, ep)
	if err != nil {
		return err
	}
	return web.JSON(w, 200, st)
}

// stream отдаёт видеофайл с поддержкой Range (перемотка). Имя файла
// берётся из подписанного токена, а не из запроса: подставить чужой путь
// нельзя, и в имени нет каталогов (проверяет catalog при сохранении).
func (a *API) stream(w http.ResponseWriter, r *http.Request) error {
	_, media, err := a.UC.VerifyStream(chi.URLParam(r, "token"))
	if err != nil {
		return err
	}
	f, err := os.Open(filepath.Join(a.MediaDir, filepath.Base(media)))
	if err != nil {
		return problem.Unavailable("media_missing", "Видеофайл не найден на сервере").With(err)
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	http.ServeContent(w, r, filepath.Base(media), st.ModTime(), f)
	return nil
}

func (a *API) progress(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		TitleID     string `json:"title_id"`
		EpisodeID   string `json:"episode_id"`
		PositionSec int    `json:"position_sec"`
		DurationSec int    `json:"duration_sec"`
		ClientMs    int64  `json:"client_ms"`
	}
	if err := web.Bind(r, &in); err != nil {
		return err
	}
	tid, err := uuid.Parse(in.TitleID)
	if err != nil {
		return problem.NotFound("Тайтл")
	}
	ep, err := optUUID(in.EpisodeID)
	if err != nil {
		return err
	}
	p, applied, err := a.UC.SaveProgress(r.Context(), tid, ep, in.PositionSec, in.DurationSec, in.ClientMs)
	if err != nil {
		return err
	}
	return web.JSON(w, 200, map[string]any{"progress": p, "applied": applied})
}

func (a *API) titleState(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	p, in, err := a.UC.ProgressFor(r.Context(), id)
	if err != nil {
		return err
	}
	return web.JSON(w, 200, map[string]any{"progress": p, "in_watchlist": in})
}

func (a *API) cont(w http.ResponseWriter, r *http.Request) error {
	list, ts, err := a.UC.Continue(r.Context())
	if err != nil {
		return err
	}
	type item struct {
		core.Progress
		Title titleRef `json:"title"`
	}
	out := make([]item, 0, len(list))
	for _, p := range list {
		t, ok := ts[p.TitleID]
		out = append(out, item{Progress: p, Title: ref(t, ok, p.TitleID)})
	}
	return web.JSON(w, 200, map[string]any{"items": out})
}

func (a *API) history(w http.ResponseWriter, r *http.Request) error {
	var cur *usecase.Cursor
	q := r.URL.Query()
	if at := q.Get("before"); at != "" {
		t, err := time.Parse(time.RFC3339Nano, at)
		id, err2 := uuid.Parse(q.Get("before_id"))
		if err != nil || err2 != nil {
			return problem.Invalid(map[string]string{"before": "Некорректный курсор"})
		}
		cur = &usecase.Cursor{At: t, ID: id}
	}
	list, ts, err := a.UC.History(r.Context(), cur)
	if err != nil {
		return err
	}
	type item struct {
		core.View
		Title titleRef `json:"title"`
	}
	out := make([]item, 0, len(list))
	for _, v := range list {
		t, ok := ts[v.TitleID]
		out = append(out, item{View: v, Title: ref(t, ok, v.TitleID)})
	}
	next := map[string]string{}
	if len(list) == 30 {
		last := list[len(list)-1]
		next = map[string]string{"before": last.StartedAt.Format(time.RFC3339Nano), "before_id": last.ID.String()}
	}
	return web.JSON(w, 200, map[string]any{"items": out, "next": next})
}

func (a *API) watchlist(w http.ResponseWriter, r *http.Request) error {
	list, ts, err := a.UC.Watchlist(r.Context())
	if err != nil {
		return err
	}
	type item struct {
		usecase.WatchItem
		Title titleRef `json:"title"`
	}
	out := make([]item, 0, len(list))
	for _, x := range list {
		t, ok := ts[x.TitleID]
		out = append(out, item{WatchItem: x, Title: ref(t, ok, x.TitleID)})
	}
	return web.JSON(w, 200, map[string]any{"items": out})
}

func (a *API) addWatch(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	if err := a.UC.AddWatch(r.Context(), id); err != nil {
		return err
	}
	return web.NoContent(w)
}

func (a *API) removeWatch(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	if err := a.UC.RemoveWatch(r.Context(), id); err != nil {
		return err
	}
	return web.NoContent(w)
}
