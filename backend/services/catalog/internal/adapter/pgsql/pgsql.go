// Package pgsql — хранилище каталога.
package pgsql

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"plenka/catalog/internal/core"
	"plenka/catalog/internal/usecase"
	"plenka/contracts/events"
	"plenka/kit/outbox"
	"plenka/kit/pgkit"
	"plenka/kit/problem"
)

type Store struct{ DB *pgxpool.Pool }

var _ usecase.Store = Store{}

const cols = `t.id, t.kind, t.name, t.original_name, t.year, t.description, t.age_rating, t.genres, t.duration_sec,
	t.in_subscription, t.price_kopecks, t.poster_hue, t.status, t.withdraw_reason, t.published_at, t.created_at,
	t.updated_at, t.revision, t.poster_rev, t.media, (SELECT count(*) FROM episodes e WHERE e.title_id = t.id)`

func scan(row pgx.Row) (core.Title, error) {
	var t core.Title
	var kind, status string
	err := row.Scan(&t.ID, &kind, &t.Name, &t.OriginalName, &t.Year, &t.Description, &t.AgeRating, &t.Genres,
		&t.DurationSec, &t.InSubscription, &t.PriceKopecks, &t.PosterHue, &status, &t.WithdrawReason, &t.PublishedAt,
		&t.CreatedAt, &t.UpdatedAt, &t.Revision, &t.PosterRev, &t.Media, &t.EpisodeCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return t, problem.NotFound("Тайтл")
	}
	t.Kind, t.Status = core.Kind(kind), core.Status(status)
	if t.Genres == nil {
		t.Genres = []string{}
	}
	return t, err
}

func episodes(ctx context.Context, q pgkit.Q, id uuid.UUID) ([]core.Episode, error) {
	rows, err := q.Query(ctx, `SELECT id, season, number, name, duration_sec, media FROM episodes WHERE title_id=$1 ORDER BY season, number`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []core.Episode{}
	for rows.Next() {
		var e core.Episode
		if err := rows.Scan(&e.ID, &e.Season, &e.Number, &e.Name, &e.DurationSec, &e.Media); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s Store) Get(ctx context.Context, id uuid.UUID) (core.Title, error) {
	t, err := scan(s.DB.QueryRow(ctx, `SELECT `+cols+` FROM titles t WHERE t.id=$1`, id))
	if err != nil {
		return t, err
	}
	t.Episodes, err = episodes(ctx, s.DB, id)
	return t, err
}

// Write — запись тайтла: READ COMMITTED + блокировка строки. Агрегат — одна
// строка (плюс его серии), конкуренция — только между администраторами,
// SERIALIZABLE здесь ничего бы не добавил, кроме повторов.
func (s Store) Write(ctx context.Context, fn func(usecase.Tx) error) error {
	return pgkit.ReadCommitted(ctx, s.DB, func(tx pgx.Tx) error { return fn(txs{tx}) })
}

type txs struct{ tx pgx.Tx }

func (t txs) Lock(ctx context.Context, id uuid.UUID) (core.Title, error) {
	ti, err := scan(t.tx.QueryRow(ctx, `SELECT `+cols+` FROM titles t WHERE t.id=$1 FOR UPDATE`, id))
	if err != nil {
		return ti, err
	}
	ti.Episodes, err = episodes(ctx, t.tx, id)
	return ti, err
}

func (t txs) Insert(ctx context.Context, x core.Title) error {
	_, err := t.tx.Exec(ctx, `INSERT INTO titles(id, kind, name, original_name, year, description, age_rating, genres, duration_sec,
		in_subscription, price_kopecks, poster_hue, status, created_at, updated_at, revision, media)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`,
		x.ID, string(x.Kind), x.Name, x.OriginalName, x.Year, x.Description, x.AgeRating, x.Genres, x.DurationSec,
		x.InSubscription, x.PriceKopecks, x.PosterHue, string(x.Status), x.CreatedAt, x.UpdatedAt, x.Revision, x.Media)
	return err
}

func (t txs) Update(ctx context.Context, x core.Title) error {
	_, err := t.tx.Exec(ctx, `UPDATE titles SET kind=$2, name=$3, original_name=$4, year=$5, description=$6, age_rating=$7,
		genres=$8, duration_sec=$9, in_subscription=$10, price_kopecks=$11, poster_hue=$12, status=$13, withdraw_reason=$14,
		published_at=$15, updated_at=$16, revision=$17, poster_rev=$18, media=$19 WHERE id=$1`,
		x.ID, string(x.Kind), x.Name, x.OriginalName, x.Year, x.Description, x.AgeRating, x.Genres, x.DurationSec,
		x.InSubscription, x.PriceKopecks, x.PosterHue, string(x.Status), x.WithdrawReason, x.PublishedAt, x.UpdatedAt, x.Revision, x.PosterRev, x.Media)
	if pgkit.Constraint(err) == "titles_published_reachable" {
		return problem.Invalid(map[string]string{"in_subscription": "Опубликованный тайтл должен быть в подписке или продаваться"})
	}
	return err
}

func (t txs) SavePoster(ctx context.Context, id uuid.UUID, contentType string, data []byte) error {
	_, err := t.tx.Exec(ctx, `INSERT INTO posters(title_id, content_type, data, updated_at) VALUES ($1,$2,$3,now())
		ON CONFLICT (title_id) DO UPDATE SET content_type=EXCLUDED.content_type, data=EXCLUDED.data, updated_at=now()`,
		id, contentType, data)
	return err
}

func (t txs) DeletePoster(ctx context.Context, id uuid.UUID) error {
	_, err := t.tx.Exec(ctx, `DELETE FROM posters WHERE title_id=$1`, id)
	return err
}

// Poster — картинка и статус тайтла (черновики видны только админу).
func (s Store) Poster(ctx context.Context, id uuid.UUID) (string, []byte, string, error) {
	var ct, status string
	var data []byte
	err := s.DB.QueryRow(ctx, `SELECT p.content_type, p.data, t.status FROM posters p JOIN titles t ON t.id = p.title_id
		WHERE p.title_id=$1`, id).Scan(&ct, &data, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil, "", problem.NotFound("Постер")
	}
	return ct, data, status, err
}

func (t txs) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := t.tx.Exec(ctx, `DELETE FROM titles WHERE id=$1`, id)
	return err
}

func (t txs) UpsertEpisode(ctx context.Context, title uuid.UUID, e core.Episode) error {
	_, err := t.tx.Exec(ctx, `INSERT INTO episodes(id, title_id, season, number, name, duration_sec, media) VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (id) DO UPDATE SET season=EXCLUDED.season, number=EXCLUDED.number, name=EXCLUDED.name,
			duration_sec=EXCLUDED.duration_sec, media=EXCLUDED.media
		WHERE episodes.title_id = EXCLUDED.title_id`, e.ID, title, e.Season, e.Number, e.Name, e.DurationSec, e.Media)
	if pgkit.Constraint(err) == "episodes_position_uq" {
		return problem.Conflict("episode_exists", fmt.Sprintf("Серия %d×%d уже есть", e.Season, e.Number))
	}
	return err
}

func (t txs) DeleteEpisode(ctx context.Context, title, ep uuid.UUID) error {
	tag, err := t.tx.Exec(ctx, `DELETE FROM episodes WHERE id=$1 AND title_id=$2`, ep, title)
	if err == nil && tag.RowsAffected() == 0 {
		return problem.NotFound("Серия")
	}
	return err
}

func (t txs) Emit(ctx context.Context, typ string, x core.Title, corr string) error {
	return outbox.Add(ctx, t.tx, events.TopicTitles, "catalog", typ, x.ID.String(), x.Revision, corr, x.Snapshot())
}

// ---------- витрина: курсорная пагинация ----------

type cursor struct {
	K  string `json:"k"`
	ID string `json:"id"`
}

func encodeCursor(k string, id uuid.UUID) string {
	b, _ := json.Marshal(cursor{K: k, ID: id.String()})
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeCursor(s string) (cursor, error) {
	var c cursor
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err == nil {
		err = json.Unmarshal(b, &c)
	}
	if err != nil || c.ID == "" {
		return c, problem.Invalid(map[string]string{"cursor": "Некорректный курсор"})
	}
	return c, nil
}

type order struct {
	col  string
	desc bool
	key  func(core.Title) string
	cast string
}

var orders = map[string]order{
	"new": {"t.published_at", true, func(t core.Title) string {
		if t.PublishedAt == nil {
			return ""
		}
		return t.PublishedAt.Format(time.RFC3339Nano)
	}, "timestamptz"},
	"updated": {"t.updated_at", true, func(t core.Title) string { return t.UpdatedAt.Format(time.RFC3339Nano) }, "timestamptz"},
	"year":    {"t.year", true, func(t core.Title) string { return fmt.Sprint(t.Year) }, "int"},
	"name":    {"t.name", false, func(t core.Title) string { return t.Name }, "text"},
}

// List — курсорная (keyset) пагинация: «дай 24 записи после такой-то».
// Стоимость не зависит от глубины прокрутки, а новые записи наверху не
// сдвигают уже показанные — для бесконечной ленты это важнее номеров страниц.
func (s Store) List(ctx context.Context, f usecase.Filter) (usecase.Page, error) {
	o, ok := orders[f.Sort]
	if !ok {
		return usecase.Page{}, problem.Invalid(map[string]string{"sort": "Неизвестная сортировка"})
	}
	var where []string
	var args []any
	arg := func(v any) string { args = append(args, v); return fmt.Sprintf("$%d", len(args)) }

	sts := make([]string, 0, len(f.Statuses))
	for _, st := range f.Statuses {
		sts = append(sts, string(st))
	}
	where = append(where, "t.status = ANY("+arg(sts)+")")
	if q := strings.TrimSpace(f.Query); q != "" {
		p := arg(q)
		// Запрос разбирается двумя конфигурациями: 'russian' стеммит русские
		// слова (и латиницу — английским стеммером), а оригинальное название
		// проиндексировано в 'simple' без стемминга. Без второй половины
		// «pirates» не нашло бы «Pirates of the Caribbean».
		where = append(where, "(t.search @@ (websearch_to_tsquery('russian', "+p+") || websearch_to_tsquery('simple', "+p+")) OR t.name ILIKE '%' || "+arg(pgkit.Like(q))+" || '%')")
	}
	if f.Kind != "" {
		where = append(where, "t.kind = "+arg(string(f.Kind)))
	}
	if f.Genre != "" {
		where = append(where, arg(f.Genre)+" = ANY(t.genres)")
	}
	if f.YearFrom > 0 {
		where = append(where, "t.year >= "+arg(f.YearFrom))
	}
	if f.YearTo > 0 {
		where = append(where, "t.year <= "+arg(f.YearTo))
	}
	if f.AgeMax >= 0 && f.AgeMax < 18 {
		where = append(where, "t.age_rating <= "+arg(f.AgeMax))
	}
	switch f.Access {
	case "subscription":
		where = append(where, "t.in_subscription")
	case "purchase":
		where = append(where, "t.price_kopecks > 0")
	}
	cmp, dir := ">", "ASC"
	if o.desc {
		cmp, dir = "<", "DESC"
	}
	if f.Cursor != "" {
		c, err := decodeCursor(f.Cursor)
		if err != nil {
			return usecase.Page{}, err
		}
		id, err := uuid.Parse(c.ID)
		if err != nil {
			return usecase.Page{}, problem.Invalid(map[string]string{"cursor": "Некорректный курсор"})
		}
		// Сравнение кортежей (col, id) — одно условие, которое использует
		// составной индекс.
		where = append(where, fmt.Sprintf("(%s, t.id) %s (%s::%s, %s)", o.col, cmp, arg(c.K), o.cast, arg(id)))
	}
	q := `SELECT ` + cols + ` FROM titles t WHERE ` + strings.Join(where, " AND ") +
		fmt.Sprintf(" ORDER BY %s %s NULLS LAST, t.id %s LIMIT %s", o.col, dir, dir, arg(f.Limit+1))
	rows, err := s.DB.Query(ctx, q, args...)
	if err != nil {
		return usecase.Page{}, err
	}
	defer rows.Close()
	items := []core.Title{}
	for rows.Next() {
		t, err := scan(rows)
		if err != nil {
			return usecase.Page{}, err
		}
		items = append(items, t)
	}
	if err := rows.Err(); err != nil {
		return usecase.Page{}, err
	}
	page := usecase.Page{Items: items}
	if len(items) > f.Limit {
		page.Items = items[:f.Limit]
		last := page.Items[f.Limit-1]
		page.Next = encodeCursor(o.key(last), last.ID)
	}
	return page, nil
}

func (s Store) GenreCounts(ctx context.Context) (map[string]int, error) {
	rows, err := s.DB.Query(ctx, `SELECT g, count(*) FROM titles, unnest(genres) g WHERE status='published' GROUP BY g`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var g string
		var n int
		if err := rows.Scan(&g, &n); err != nil {
			return nil, err
		}
		out[g] = n
	}
	return out, rows.Err()
}
