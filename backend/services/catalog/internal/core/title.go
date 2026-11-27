// Package core — тайтл каталога и его жизненный цикл.
package core

import (
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"plenka/contracts/events"
	"plenka/kit/problem"
)

type Kind string

const (
	Film   Kind = "film"
	Series Kind = "series"
)

type Status string

const (
	Draft     Status = "draft"
	Published Status = "published"
	Withdrawn Status = "withdrawn"
)

// AgeRatings — шкала возрастной маркировки (436-ФЗ).
var AgeRatings = []int{0, 6, 12, 16, 18}

// Genres — закрытый справочник: свободный ввод дал бы «драма», «Драма»,
// «драмма» и сломал фильтр.
var Genres = []string{"боевик", "детектив", "драма", "комедия", "мелодрама", "мультфильм",
	"документальный", "приключения", "триллер", "ужасы", "фантастика", "фэнтези", "семейный", "исторический"}

type Episode struct {
	ID          uuid.UUID
	Season      int
	Number      int
	Name        string
	DurationSec int
	Media       string
}

// Fields — редактируемая часть тайтла.
type Fields struct {
	Kind           Kind
	Name           string
	OriginalName   string
	Year           int
	Description    string
	AgeRating      int
	Genres         []string
	DurationSec    int
	InSubscription bool
	PriceKopecks   int64
	PosterHue      int
	Media          string // видеофайл фильма в хранилище плеера, пусто — демо-ролик
}

func (f Fields) Clean(now time.Time) (Fields, error) {
	e := problem.Fields{}
	f.Name = strings.Join(strings.Fields(f.Name), " ")
	f.OriginalName = strings.Join(strings.Fields(f.OriginalName), " ")
	f.Description = strings.TrimSpace(f.Description)
	if f.Kind != Film && f.Kind != Series {
		e.Set("kind", "Выберите: фильм или сериал")
	}
	if n := utf8.RuneCountInString(f.Name); n < 1 || n > 200 {
		e.Set("name", "Название — от 1 до 200 символов")
	}
	if utf8.RuneCountInString(f.OriginalName) > 200 {
		e.Set("original_name", "Не длиннее 200 символов")
	}
	if f.Year < 1895 || f.Year > now.Year()+2 {
		e.Set("year", "Год — от 1895 до ближайших лет")
	}
	if utf8.RuneCountInString(f.Description) > 4000 {
		e.Set("description", "Описание — до 4000 символов")
	}
	if !slices.Contains(AgeRatings, f.AgeRating) {
		e.Set("age_rating", "Возрастной рейтинг: 0+, 6+, 12+, 16+ или 18+")
	}
	seen := map[string]bool{}
	gs := []string{} // не nil: в БД колонка NOT NULL
	for _, g := range f.Genres {
		g = strings.ToLower(strings.TrimSpace(g))
		if !slices.Contains(Genres, g) {
			e.Set("genres", "Неизвестный жанр: "+g)
			continue
		}
		if !seen[g] {
			seen[g] = true
			gs = append(gs, g)
		}
	}
	if len(gs) > 4 {
		e.Set("genres", "Не больше 4 жанров")
	}
	f.Genres = gs
	if f.Kind == Film && (f.DurationSec < 60 || f.DurationSec > 6*3600) {
		e.Set("duration_sec", "Длительность фильма — от 1 минуты до 6 часов")
	}
	if f.Kind == Series {
		f.DurationSec = 0
		if f.PriceKopecks != 0 {
			e.Set("price_kopecks", "Сериалы доступны только по подписке")
		}
	}
	if f.PriceKopecks < 0 || f.PriceKopecks > 100_000_00 {
		e.Set("price_kopecks", "Цена — от 0 до 100 000 ₽")
	}
	if f.PriceKopecks > 0 && f.PriceKopecks < 49_00 {
		e.Set("price_kopecks", "Минимальная цена покупки — 49 ₽")
	}
	f.PosterHue = ((f.PosterHue % 360) + 360) % 360
	f.Media = strings.ToLower(strings.TrimSpace(f.Media))
	if !ValidMedia(f.Media) {
		e.Set("media", "Имя файла — латиница, цифры, точка, дефис, на конце .mp4")
	}
	return f, e.Err()
}

type Title struct {
	ID uuid.UUID
	Fields
	Status         Status
	WithdrawReason string
	PublishedAt    *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
	Revision       int64
	PosterRev      int64 // 0 — постера нет, иначе ревизия, на которой он загружен
	Episodes       []Episode
	EpisodeCount   int // в списках серии не грузятся, только их число
}

func NewDraft(f Fields, now time.Time) (Title, error) {
	f, err := f.Clean(now)
	if err != nil {
		return Title{}, err
	}
	return Title{ID: uuid.New(), Fields: f, Status: Draft, CreatedAt: now, UpdatedAt: now, Revision: 1}, nil
}

// Edit — тип (фильм/сериал) после публикации не меняется: на него уже
// ссылаются покупки и история просмотров.
func (t *Title) Edit(f Fields, now time.Time) error {
	f, err := f.Clean(now)
	if err != nil {
		return err
	}
	if t.Status != Draft && f.Kind != t.Kind {
		return problem.Invalid(map[string]string{"kind": "Тип нельзя менять после публикации"})
	}
	t.Fields, t.UpdatedAt = f, now
	return nil
}

// readiness — что мешает опубликовать.
func (t Title) readiness() problem.Fields {
	e := problem.Fields{}
	if utf8.RuneCountInString(t.Description) < 20 {
		e.Set("description", "Перед публикацией добавьте описание (от 20 символов)")
	}
	if !t.InSubscription && t.PriceKopecks == 0 {
		e.Set("in_subscription", "Включите подписку или задайте цену покупки")
	}
	if t.Kind == Series && len(t.Episodes) == 0 {
		e.Set("episodes", "Добавьте хотя бы одну серию")
	}
	return e
}

// Publish: draft → published или withdrawn → published.
func (t *Title) Publish(now time.Time) error {
	if t.Status == Published {
		return problem.Conflict("already_published", "Тайтл уже опубликован")
	}
	if err := t.readiness().Err(); err != nil {
		return err
	}
	t.Status, t.WithdrawReason, t.UpdatedAt = Published, "", now
	if t.PublishedAt == nil {
		t.PublishedAt = &now // дата первой публикации не меняется при возврате
	}
	return nil
}

// Withdraw — снять с показа (истёк договор с правообладателем и т. п.).
func (t *Title) Withdraw(reason string, now time.Time) error {
	if t.Status != Published {
		return problem.Conflict("not_published", "Снять с показа можно только опубликованный тайтл")
	}
	reason = strings.TrimSpace(reason)
	if n := utf8.RuneCountInString(reason); n < 3 || n > 300 {
		return problem.Invalid(map[string]string{"reason": "Укажите причину (3–300 символов)"})
	}
	t.Status, t.WithdrawReason, t.UpdatedAt = Withdrawn, reason, now
	return nil
}

// CanDelete — удалять можно только черновик: всё, что было на витрине,
// могло быть куплено.
func (t Title) CanDelete() error {
	if t.Status != Draft {
		return problem.Conflict("not_draft", "Удалить можно только черновик — опубликованный тайтл снимите с показа")
	}
	return nil
}

// Visible — что делать с событием: витрина видит только published/withdrawn.
func (t Title) Snapshot() events.TitleSnapshot {
	eps := make([]events.Episode, 0, len(t.Episodes))
	for _, e := range t.Episodes {
		eps = append(eps, events.Episode{ID: e.ID.String(), Season: e.Season, Number: e.Number, Name: e.Name, DurationSec: e.DurationSec, Media: e.Media})
	}
	return events.TitleSnapshot{
		ID: t.ID.String(), Kind: string(t.Kind), Name: t.Name, Year: t.Year, AgeRating: t.AgeRating,
		Status: string(t.Status), InSubscription: t.InSubscription, PriceKopecks: t.PriceKopecks,
		DurationSec: t.DurationSec, Episodes: eps, Revision: t.Revision, PosterHue: t.PosterHue, PosterRev: t.PosterRev, Media: t.Media,
	}
}

// CleanEpisode проверяет серию.
func CleanEpisode(e Episode) (Episode, error) {
	f := problem.Fields{}
	e.Name = strings.Join(strings.Fields(e.Name), " ")
	if e.Season < 1 || e.Season > 99 {
		f.Set("season", "Сезон — от 1 до 99")
	}
	if e.Number < 1 || e.Number > 999 {
		f.Set("number", "Номер серии — от 1 до 999")
	}
	if n := utf8.RuneCountInString(e.Name); n < 1 || n > 200 {
		f.Set("name", "Название серии — от 1 до 200 символов")
	}
	if e.DurationSec < 60 || e.DurationSec > 4*3600 {
		f.Set("duration_sec", "Длительность серии — от 1 минуты до 4 часов")
	}
	e.Media = strings.ToLower(strings.TrimSpace(e.Media))
	if !ValidMedia(e.Media) {
		f.Set("media", "Имя файла — латиница, цифры, точка, дефис, на конце .mp4")
	}
	return e, f.Err()
}

var mediaRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,120}\.mp4$`)

// ValidMedia — пустое имя или безопасное имя файла: без каталогов и «..».
func ValidMedia(m string) bool { return m == "" || mediaRe.MatchString(m) }
