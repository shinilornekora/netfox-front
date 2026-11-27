// Package usecase — сценарии каталога.
package usecase

import (
	"context"
	"time"

	"github.com/google/uuid"

	"plenka/catalog/internal/core"
	"plenka/contracts/events"
	"plenka/kit/principal"
	"plenka/kit/problem"
)

// Filter — параметры публичной витрины.
type Filter struct {
	Query    string
	Kind     core.Kind
	Genre    string
	YearFrom int
	YearTo   int
	AgeMax   int    // -1 — без ограничения
	Access   string // "", "subscription", "purchase"
	Sort     string // new | year | name
	Cursor   string
	Limit    int
	Statuses []core.Status // для админа
}

type Page struct {
	Items []core.Title
	Next  string // пусто — дальше ничего нет
}

// Tx — операции внутри одной транзакции записи.
type Tx interface {
	Lock(ctx context.Context, id uuid.UUID) (core.Title, error) // SELECT … FOR UPDATE + серии
	Insert(ctx context.Context, t core.Title) error
	Update(ctx context.Context, t core.Title) error
	Delete(ctx context.Context, id uuid.UUID) error
	UpsertEpisode(ctx context.Context, title uuid.UUID, e core.Episode) error
	DeleteEpisode(ctx context.Context, title, episode uuid.UUID) error
	SavePoster(ctx context.Context, id uuid.UUID, contentType string, data []byte) error
	DeletePoster(ctx context.Context, id uuid.UUID) error
	Emit(ctx context.Context, typ string, t core.Title, correlation string) error
}

type Store interface {
	Write(ctx context.Context, fn func(Tx) error) error
	Get(ctx context.Context, id uuid.UUID) (core.Title, error)
	List(ctx context.Context, f Filter) (Page, error)
	GenreCounts(ctx context.Context) (map[string]int, error)
	Poster(ctx context.Context, id uuid.UUID) (contentType string, data []byte, status string, err error)
}

type Service struct {
	Store Store
	Clock func() time.Time
}

func (s *Service) now() time.Time {
	if s.Clock != nil {
		return s.Clock().UTC()
	}
	return time.Now().UTC()
}

// ---------- витрина (все) ----------

func (s *Service) Browse(ctx context.Context, f Filter) (Page, error) {
	if f.Limit <= 0 || f.Limit > 48 {
		f.Limit = 24
	}
	switch f.Sort {
	case "":
		f.Sort = "new"
	case "new", "year", "name":
	default:
		return Page{}, problem.Invalid(map[string]string{"sort": "Сортировка: new, year или name"})
	}
	if f.Kind != "" && f.Kind != core.Film && f.Kind != core.Series {
		return Page{}, problem.Invalid(map[string]string{"kind": "film или series"})
	}
	f.Statuses = []core.Status{core.Published}
	return s.Store.List(ctx, f)
}

// Title — карточка. Неопубликованное видит только администратор.
func (s *Service) Title(ctx context.Context, id uuid.UUID) (core.Title, error) {
	t, err := s.Store.Get(ctx, id)
	if err != nil {
		return t, err
	}
	if t.Status != core.Published {
		if p, ok := principal.From(ctx); !ok || p.Role != principal.Admin {
			return core.Title{}, problem.NotFound("Тайтл")
		}
	}
	return t, nil
}

func (s *Service) Genres(ctx context.Context) ([]GenreCount, error) {
	counts, err := s.Store.GenreCounts(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]GenreCount, 0, len(core.Genres))
	for _, g := range core.Genres {
		out = append(out, GenreCount{Name: g, Count: counts[g]})
	}
	return out, nil
}

type GenreCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// ---------- администрирование ----------

func admin(ctx context.Context) error {
	_, err := principal.Need(ctx, principal.Admin)
	return err
}

func (s *Service) AdminList(ctx context.Context, f Filter) (Page, error) {
	if err := admin(ctx); err != nil {
		return Page{}, err
	}
	if f.Limit <= 0 || f.Limit > 100 {
		f.Limit = 30
	}
	if f.Sort == "" {
		f.Sort = "updated"
	}
	if len(f.Statuses) == 0 {
		f.Statuses = []core.Status{core.Draft, core.Published, core.Withdrawn}
	}
	f.AgeMax = -1
	return s.Store.List(ctx, f)
}

func (s *Service) Create(ctx context.Context, f core.Fields) (core.Title, error) {
	if err := admin(ctx); err != nil {
		return core.Title{}, err
	}
	t, err := core.NewDraft(f, s.now())
	if err != nil {
		return t, err
	}
	return t, s.Store.Write(ctx, func(tx Tx) error { return tx.Insert(ctx, t) })
}

// mutate — общий каркас изменения: блокировка строки, проверка ревизии,
// изменение, запись, событие (если тайтл виден на витрине).
func (s *Service) mutate(ctx context.Context, id uuid.UUID, rev int64, fn func(t *core.Title, tx Tx) (event string, err error)) (core.Title, error) {
	if err := admin(ctx); err != nil {
		return core.Title{}, err
	}
	var out core.Title
	err := s.Store.Write(ctx, func(tx Tx) error {
		t, err := tx.Lock(ctx, id)
		if err != nil {
			return err
		}
		if rev != 0 && rev != t.Revision {
			return problem.Conflict("stale", "Тайтл изменили в другой вкладке — обновите страницу")
		}
		ev, err := fn(&t, tx)
		if err != nil {
			return err
		}
		t.Revision++
		if err := tx.Update(ctx, t); err != nil {
			return err
		}
		if ev != "" {
			if err := tx.Emit(ctx, ev, t, ""); err != nil {
				return err
			}
		}
		out = t
		return nil
	})
	return out, err
}

func (s *Service) Edit(ctx context.Context, id uuid.UUID, f core.Fields, rev int64) (core.Title, error) {
	return s.mutate(ctx, id, rev, func(t *core.Title, _ Tx) (string, error) {
		if err := t.Edit(f, s.now()); err != nil {
			return "", err
		}
		if t.Status == core.Published {
			return events.TitleUpdated, nil
		}
		return "", nil
	})
}

func (s *Service) Publish(ctx context.Context, id uuid.UUID, rev int64) (core.Title, error) {
	return s.mutate(ctx, id, rev, func(t *core.Title, _ Tx) (string, error) {
		return events.TitlePublished, t.Publish(s.now())
	})
}

func (s *Service) Withdraw(ctx context.Context, id uuid.UUID, reason string, rev int64) (core.Title, error) {
	return s.mutate(ctx, id, rev, func(t *core.Title, _ Tx) (string, error) {
		return events.TitleWithdrawn, t.Withdraw(reason, s.now())
	})
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	if err := admin(ctx); err != nil {
		return err
	}
	return s.Store.Write(ctx, func(tx Tx) error {
		t, err := tx.Lock(ctx, id)
		if err != nil {
			return err
		}
		if err := t.CanDelete(); err != nil {
			return err
		}
		return tx.Delete(ctx, id)
	})
}

// SaveEpisode — добавить или изменить серию сериала.
func (s *Service) SaveEpisode(ctx context.Context, titleID uuid.UUID, e core.Episode) (core.Title, error) {
	e, err := core.CleanEpisode(e)
	if err != nil {
		return core.Title{}, err
	}
	return s.mutate(ctx, titleID, 0, func(t *core.Title, tx Tx) (string, error) {
		if t.Kind != core.Series {
			return "", problem.Conflict("not_series", "Серии бывают только у сериалов")
		}
		if e.ID == uuid.Nil {
			e.ID = uuid.New()
		}
		if err := tx.UpsertEpisode(ctx, t.ID, e); err != nil {
			return "", err
		}
		t.Episodes = upsert(t.Episodes, e)
		t.UpdatedAt = s.now()
		if t.Status == core.Published {
			return events.TitleUpdated, nil
		}
		return "", nil
	})
}

func (s *Service) DeleteEpisode(ctx context.Context, titleID, episodeID uuid.UUID) (core.Title, error) {
	return s.mutate(ctx, titleID, 0, func(t *core.Title, tx Tx) (string, error) {
		if t.Status == core.Published && len(t.Episodes) <= 1 {
			return "", problem.Conflict("last_episode", "У опубликованного сериала должна остаться хотя бы одна серия")
		}
		if err := tx.DeleteEpisode(ctx, t.ID, episodeID); err != nil {
			return "", err
		}
		out := t.Episodes[:0]
		for _, x := range t.Episodes {
			if x.ID != episodeID {
				out = append(out, x)
			}
		}
		t.Episodes = out
		t.UpdatedAt = s.now()
		if t.Status == core.Published {
			return events.TitleUpdated, nil
		}
		return "", nil
	})
}

func upsert(list []core.Episode, e core.Episode) []core.Episode {
	for i := range list {
		if list[i].ID == e.ID {
			list[i] = e
			return list
		}
	}
	return append(list, e)
}

// MaxPoster — предел размера постера.
const MaxPoster = 3 << 20

var posterTypes = map[string]bool{"image/jpeg": true, "image/png": true, "image/webp": true}

// SetPoster заменяет постер. Тип определяется по содержимому, а не по
// заголовку запроса: переименованный .exe постером не станет.
func (s *Service) SetPoster(ctx context.Context, id uuid.UUID, contentType string, data []byte) (core.Title, error) {
	if len(data) == 0 || len(data) > MaxPoster {
		return core.Title{}, problem.Invalid(map[string]string{"poster": "Постер — картинка до 3 МБ"})
	}
	if !posterTypes[contentType] {
		return core.Title{}, problem.Invalid(map[string]string{"poster": "Поддерживаются JPEG, PNG и WebP"})
	}
	return s.mutate(ctx, id, 0, func(t *core.Title, tx Tx) (string, error) {
		if err := tx.SavePoster(ctx, id, contentType, data); err != nil {
			return "", err
		}
		t.PosterRev = t.Revision + 1
		t.UpdatedAt = s.now()
		if t.Status == core.Published {
			return events.TitleUpdated, nil
		}
		return "", nil
	})
}

func (s *Service) DeletePoster(ctx context.Context, id uuid.UUID) (core.Title, error) {
	return s.mutate(ctx, id, 0, func(t *core.Title, tx Tx) (string, error) {
		if err := tx.DeletePoster(ctx, id); err != nil {
			return "", err
		}
		t.PosterRev = 0
		t.UpdatedAt = s.now()
		if t.Status == core.Published {
			return events.TitleUpdated, nil
		}
		return "", nil
	})
}

// Poster отдаёт картинку. Постер черновика видит только администратор.
func (s *Service) Poster(ctx context.Context, id uuid.UUID) (string, []byte, error) {
	ct, data, status, err := s.Store.Poster(ctx, id)
	if err != nil {
		return "", nil, err
	}
	if status != string(core.Published) && admin(ctx) != nil {
		return "", nil, problem.NotFound("Постер")
	}
	return ct, data, nil
}
