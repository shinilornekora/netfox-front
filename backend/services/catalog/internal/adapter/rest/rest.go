// Package rest — HTTP API каталога.
package rest

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"plenka/catalog/internal/core"
	"plenka/catalog/internal/usecase"
	"plenka/kit/principal"
	"plenka/kit/problem"
	"plenka/kit/web"
)

type API struct{ UC *usecase.Service }

func (a *API) Mount(r chi.Router, h func(web.Handler) http.HandlerFunc) {
	r.Get("/api/titles", h(a.browse))
	r.Get("/api/titles/{id}", h(a.title))
	r.Get("/api/titles/{id}/poster", h(a.poster))
	r.Get("/api/genres", h(a.genres))
	r.Route("/api/admin/titles", func(r chi.Router) {
		r.Get("/", h(a.adminList))
		r.Post("/", h(a.create))
		r.Put("/{id}", h(a.edit))
		r.Delete("/{id}", h(a.remove))
		r.Post("/{id}/publish", h(a.publish))
		r.Post("/{id}/withdraw", h(a.withdraw))
		r.Put("/{id}/episodes", h(a.saveEpisode))
		r.Delete("/{id}/episodes/{eid}", h(a.deleteEpisode))
		r.Put("/{id}/poster", h(a.putPoster))
		r.Delete("/{id}/poster", h(a.deletePoster))
	})
}

type episodeDTO struct {
	ID          string `json:"id"`
	Season      int    `json:"season"`
	Number      int    `json:"number"`
	Name        string `json:"name"`
	DurationSec int    `json:"duration_sec"`
	Media       string `json:"media"`
}

type titleDTO struct {
	ID             string       `json:"id"`
	PosterURL      string       `json:"poster_url,omitempty"`
	Media          string       `json:"media"`
	Kind           string       `json:"kind"`
	Name           string       `json:"name"`
	OriginalName   string       `json:"original_name"`
	Year           int          `json:"year"`
	Description    string       `json:"description"`
	AgeRating      int          `json:"age_rating"`
	Genres         []string     `json:"genres"`
	DurationSec    int          `json:"duration_sec"`
	InSubscription bool         `json:"in_subscription"`
	PriceKopecks   int64        `json:"price_kopecks"`
	PosterHue      int          `json:"poster_hue"`
	Status         string       `json:"status"`
	WithdrawReason string       `json:"withdraw_reason,omitempty"`
	PublishedAt    *time.Time   `json:"published_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
	Revision       int64        `json:"revision"`
	EpisodeCount   int          `json:"episode_count"`
	Episodes       []episodeDTO `json:"episodes,omitempty"`
}

func dto(t core.Title, withEpisodes bool) titleDTO {
	d := titleDTO{ID: t.ID.String(), Kind: string(t.Kind), Name: t.Name, OriginalName: t.OriginalName, Year: t.Year,
		Description: t.Description, AgeRating: t.AgeRating, Genres: t.Genres, DurationSec: t.DurationSec,
		InSubscription: t.InSubscription, PriceKopecks: t.PriceKopecks, PosterHue: t.PosterHue, Status: string(t.Status),
		WithdrawReason: t.WithdrawReason, PublishedAt: t.PublishedAt, UpdatedAt: t.UpdatedAt, Revision: t.Revision,
		EpisodeCount: t.EpisodeCount, Media: t.Media}
	if t.PosterRev > 0 {
		d.PosterURL = fmt.Sprintf("/api/titles/%s/poster?v=%d", t.ID, t.PosterRev)
	}
	if withEpisodes {
		d.Episodes = []episodeDTO{}
		for _, e := range t.Episodes {
			d.Episodes = append(d.Episodes, episodeDTO{ID: e.ID.String(), Season: e.Season, Number: e.Number, Name: e.Name, DurationSec: e.DurationSec, Media: e.Media})
		}
		d.EpisodeCount = len(t.Episodes)
	}
	return d
}

func page(w http.ResponseWriter, p usecase.Page) error {
	items := make([]titleDTO, 0, len(p.Items))
	for _, t := range p.Items {
		items = append(items, dto(t, false))
	}
	return web.JSON(w, 200, map[string]any{"items": items, "next_cursor": p.Next})
}

func filter(r *http.Request) usecase.Filter {
	q := r.URL.Query()
	atoi := func(k string) int { n, _ := strconv.Atoi(q.Get(k)); return n }
	age := -1
	if v := q.Get("age_max"); v != "" {
		age = atoi("age_max")
	}
	f := usecase.Filter{Query: q.Get("q"), Kind: core.Kind(q.Get("kind")), Genre: strings.ToLower(q.Get("genre")),
		YearFrom: atoi("year_from"), YearTo: atoi("year_to"), AgeMax: age, Access: q.Get("access"),
		Sort: q.Get("sort"), Cursor: q.Get("cursor"), Limit: atoi("limit")}
	// Детский профиль: если зрителю меньше 18, витрина сама скрывает то, что
	// ему смотреть нельзя, даже если фильтр не выбран.
	if p, ok := principal.From(r.Context()); ok && p.Role == principal.Viewer {
		if age := p.Age(time.Now()); age > 0 && age < 18 {
			limit := 0
			for _, rt := range core.AgeRatings {
				if rt <= age {
					limit = rt
				}
			}
			if f.AgeMax < 0 || f.AgeMax > limit {
				f.AgeMax = limit
			}
		}
	}
	for _, st := range q["status"] {
		f.Statuses = append(f.Statuses, core.Status(st))
	}
	return f
}

func id(r *http.Request, name string) (uuid.UUID, error) {
	v, err := uuid.Parse(chi.URLParam(r, name))
	if err != nil {
		return uuid.Nil, problem.NotFound("Тайтл")
	}
	return v, nil
}

func (a *API) browse(w http.ResponseWriter, r *http.Request) error {
	p, err := a.UC.Browse(r.Context(), filter(r))
	if err != nil {
		return err
	}
	return page(w, p)
}

func (a *API) title(w http.ResponseWriter, r *http.Request) error {
	tid, err := id(r, "id")
	if err != nil {
		return err
	}
	t, err := a.UC.Title(r.Context(), tid)
	if err != nil {
		return err
	}
	return web.JSON(w, 200, dto(t, true))
}

func (a *API) genres(w http.ResponseWriter, r *http.Request) error {
	g, err := a.UC.Genres(r.Context())
	if err != nil {
		return err
	}
	w.Header().Set("Cache-Control", "public, max-age=60")
	return web.JSON(w, 200, map[string]any{"items": g})
}

func (a *API) adminList(w http.ResponseWriter, r *http.Request) error {
	p, err := a.UC.AdminList(r.Context(), filter(r))
	if err != nil {
		return err
	}
	return page(w, p)
}

type fieldsIn struct {
	Kind           string   `json:"kind"`
	Name           string   `json:"name"`
	OriginalName   string   `json:"original_name"`
	Year           int      `json:"year"`
	Description    string   `json:"description"`
	AgeRating      int      `json:"age_rating"`
	Genres         []string `json:"genres"`
	DurationSec    int      `json:"duration_sec"`
	InSubscription bool     `json:"in_subscription"`
	PriceKopecks   int64    `json:"price_kopecks"`
	PosterHue      int      `json:"poster_hue"`
	Media          string   `json:"media"`
	Revision       int64    `json:"revision"`
}

func (in fieldsIn) core() core.Fields {
	return core.Fields{Kind: core.Kind(in.Kind), Name: in.Name, OriginalName: in.OriginalName, Year: in.Year,
		Description: in.Description, AgeRating: in.AgeRating, Genres: in.Genres, DurationSec: in.DurationSec,
		InSubscription: in.InSubscription, PriceKopecks: in.PriceKopecks, PosterHue: in.PosterHue, Media: in.Media}
}

func (a *API) create(w http.ResponseWriter, r *http.Request) error {
	var in fieldsIn
	if err := web.Bind(r, &in); err != nil {
		return err
	}
	t, err := a.UC.Create(r.Context(), in.core())
	if err != nil {
		return err
	}
	return web.JSON(w, http.StatusCreated, dto(t, true))
}

func (a *API) edit(w http.ResponseWriter, r *http.Request) error {
	tid, err := id(r, "id")
	if err != nil {
		return err
	}
	var in fieldsIn
	if err := web.Bind(r, &in); err != nil {
		return err
	}
	t, err := a.UC.Edit(r.Context(), tid, in.core(), in.Revision)
	if err != nil {
		return err
	}
	return web.JSON(w, 200, dto(t, true))
}

func (a *API) publish(w http.ResponseWriter, r *http.Request) error {
	tid, err := id(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		Revision int64 `json:"revision"`
	}
	_ = web.Bind(r, &in)
	t, err := a.UC.Publish(r.Context(), tid, in.Revision)
	if err != nil {
		return err
	}
	return web.JSON(w, 200, dto(t, true))
}

func (a *API) withdraw(w http.ResponseWriter, r *http.Request) error {
	tid, err := id(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		Reason   string `json:"reason"`
		Revision int64  `json:"revision"`
	}
	if err := web.Bind(r, &in); err != nil {
		return err
	}
	t, err := a.UC.Withdraw(r.Context(), tid, in.Reason, in.Revision)
	if err != nil {
		return err
	}
	return web.JSON(w, 200, dto(t, true))
}

func (a *API) remove(w http.ResponseWriter, r *http.Request) error {
	tid, err := id(r, "id")
	if err != nil {
		return err
	}
	if err := a.UC.Delete(r.Context(), tid); err != nil {
		return err
	}
	return web.NoContent(w)
}

func (a *API) saveEpisode(w http.ResponseWriter, r *http.Request) error {
	tid, err := id(r, "id")
	if err != nil {
		return err
	}
	var in episodeDTO
	if err := web.Bind(r, &in); err != nil {
		return err
	}
	e := core.Episode{Season: in.Season, Number: in.Number, Name: in.Name, DurationSec: in.DurationSec, Media: in.Media}
	if in.ID != "" {
		if e.ID, err = uuid.Parse(in.ID); err != nil {
			return problem.NotFound("Серия")
		}
	}
	t, err := a.UC.SaveEpisode(r.Context(), tid, e)
	if err != nil {
		return err
	}
	return web.JSON(w, 200, dto(t, true))
}

func (a *API) deleteEpisode(w http.ResponseWriter, r *http.Request) error {
	tid, err := id(r, "id")
	if err != nil {
		return err
	}
	eid, err := id(r, "eid")
	if err != nil {
		return err
	}
	t, err := a.UC.DeleteEpisode(r.Context(), tid, eid)
	if err != nil {
		return err
	}
	return web.JSON(w, 200, dto(t, true))
}

// poster: адрес содержит ?v=<ревизия постера>, поэтому ответ неизменяем
// и кэшируется на год; новый постер получит новый адрес.
func (a *API) poster(w http.ResponseWriter, r *http.Request) error {
	tid, err := id(r, "id")
	if err != nil {
		return err
	}
	ct, data, err := a.UC.Poster(r.Context(), tid)
	if err != nil {
		return err
	}
	h := w.Header()
	h.Set("Content-Type", ct)
	if r.URL.Query().Get("v") != "" {
		h.Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		h.Set("Cache-Control", "public, max-age=300")
	}
	h.Set("Content-Length", strconv.Itoa(len(data)))
	_, err = w.Write(data)
	return err
}

func (a *API) putPoster(w http.ResponseWriter, r *http.Request) error {
	tid, err := id(r, "id")
	if err != nil {
		return err
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, usecase.MaxPoster+1))
	if err != nil {
		return problem.Invalid(map[string]string{"poster": "Постер — картинка до 3 МБ"})
	}
	t, err := a.UC.SetPoster(r.Context(), tid, http.DetectContentType(data), data)
	if err != nil {
		return err
	}
	return web.JSON(w, 200, dto(t, true))
}

func (a *API) deletePoster(w http.ResponseWriter, r *http.Request) error {
	tid, err := id(r, "id")
	if err != nil {
		return err
	}
	t, err := a.UC.DeletePoster(r.Context(), tid)
	if err != nil {
		return err
	}
	return web.JSON(w, 200, dto(t, true))
}
