// Package pgsql — хранилище playback. Все данные зрителя читаются и
// пишутся только в транзакции с SET LOCAL plenka.user_id — иначе RLS
// вернёт пустоту.
package pgsql

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"plenka/contracts/events"
	"plenka/kit/outbox"
	"plenka/kit/pgkit"
	"plenka/playback/internal/core"
	"plenka/playback/internal/usecase"
)

type Store struct{ DB *pgxpool.Pool }

var _ usecase.Store = Store{}

func (s Store) as(ctx context.Context, user uuid.UUID, fn func(pgx.Tx) error) error {
	return pgkit.ReadCommitted(ctx, s.DB, func(tx pgx.Tx) error {
		if err := pgkit.AsUser(ctx, tx, user.String()); err != nil {
			return err
		}
		return fn(tx)
	})
}

const titleCols = `id, name, kind, year, age_rating, status, duration_sec, poster_hue, poster_rev, media, episodes`

func scanTitle(row pgx.Row) (core.Title, error) {
	var t core.Title
	var eps []byte
	if err := row.Scan(&t.ID, &t.Name, &t.Kind, &t.Year, &t.AgeRating, &t.Status, &t.DurationSec, &t.PosterHue, &t.PosterRev, &t.Media, &eps); err != nil {
		return t, err
	}
	err := json.Unmarshal(eps, &t.Episodes)
	return t, err
}

func (s Store) Title(ctx context.Context, id uuid.UUID) (core.Title, bool, error) {
	t, err := scanTitle(s.DB.QueryRow(ctx, `SELECT `+titleCols+` FROM title_replica WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return t, false, nil
	}
	return t, err == nil, err
}

func (s Store) Titles(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]core.Title, error) {
	out := map[uuid.UUID]core.Title{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := s.DB.Query(ctx, `SELECT `+titleCols+` FROM title_replica WHERE id = ANY($1)`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		t, err := scanTitle(rows)
		if err != nil {
			return nil, err
		}
		out[t.ID] = t
	}
	return out, rows.Err()
}

func (s Store) ApplyTitle(ctx context.Context, env events.Envelope, t events.TitleSnapshot) error {
	eps, err := json.Marshal(t.Episodes)
	if err != nil {
		return err
	}
	return pgkit.ReadCommitted(ctx, s.DB, func(tx pgx.Tx) error {
		dup, err := outbox.Seen(ctx, tx, env)
		if err != nil || dup {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO title_replica(`+titleCols+`, revision) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
			ON CONFLICT (id) DO UPDATE SET name=EXCLUDED.name, kind=EXCLUDED.kind, year=EXCLUDED.year, age_rating=EXCLUDED.age_rating,
				status=EXCLUDED.status, duration_sec=EXCLUDED.duration_sec, poster_hue=EXCLUDED.poster_hue, poster_rev=EXCLUDED.poster_rev, media=EXCLUDED.media,
				episodes=EXCLUDED.episodes, revision=EXCLUDED.revision
			WHERE title_replica.revision < EXCLUDED.revision`,
			t.ID, t.Name, t.Kind, t.Year, t.AgeRating, t.Status, t.DurationSec, t.PosterHue, t.PosterRev, t.Media, eps, t.Revision)
		return err
	})
}

func (s Store) RecordView(ctx context.Context, user uuid.UUID, v core.View) error {
	return s.as(ctx, user, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO views(id, user_id, title_id, episode_id, via, started_at) VALUES ($1,$2,$3,$4,$5,$6)`,
			v.ID, user, v.TitleID, v.EpisodeID, v.Via, v.StartedAt)
		return err
	})
}

const progCols = `title_id, episode_id, position_sec, duration_sec, finished, client_ms, updated_at`

func scanProg(row pgx.Row) (core.Progress, error) {
	var p core.Progress
	err := row.Scan(&p.TitleID, &p.EpisodeID, &p.PositionSec, &p.DurationSec, &p.Finished, &p.ClientMs, &p.UpdatedAt)
	return p, err
}

func (s Store) Progress(ctx context.Context, user, title uuid.UUID) (*core.Progress, error) {
	var out *core.Progress
	err := s.as(ctx, user, func(tx pgx.Tx) error {
		p, err := scanProg(tx.QueryRow(ctx, `SELECT `+progCols+` FROM progress WHERE user_id=$1 AND title_id=$2`, user, title))
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err == nil {
			out = &p
		}
		return err
	})
	return out, err
}

// SaveProgress — upsert с защитой от устаревших пингов: строка обновляется,
// только если время устройства у нового пинга больше. Один SQL-оператор —
// без отдельного SELECT и без гонки «прочитал–перезаписал».
func (s Store) SaveProgress(ctx context.Context, user uuid.UUID, p core.Progress) (bool, error) {
	applied := false
	err := s.as(ctx, user, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `INSERT INTO progress(user_id, title_id, episode_id, position_sec, duration_sec, finished, client_ms, updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
			ON CONFLICT (user_id, title_id) DO UPDATE SET episode_id=EXCLUDED.episode_id, position_sec=EXCLUDED.position_sec,
				duration_sec=EXCLUDED.duration_sec, finished=EXCLUDED.finished, client_ms=EXCLUDED.client_ms, updated_at=EXCLUDED.updated_at
			WHERE progress.client_ms < EXCLUDED.client_ms`,
			user, p.TitleID, p.EpisodeID, p.PositionSec, p.DurationSec, p.Finished, p.ClientMs, p.UpdatedAt)
		applied = tag.RowsAffected() == 1
		return err
	})
	return applied, err
}

func (s Store) Continue(ctx context.Context, user uuid.UUID, limit int) ([]core.Progress, error) {
	out := []core.Progress{}
	err := s.as(ctx, user, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+progCols+` FROM progress p
			WHERE user_id=$1 AND NOT finished
			  AND EXISTS (SELECT 1 FROM title_replica t WHERE t.id = p.title_id AND t.status='published')
			ORDER BY updated_at DESC LIMIT $2`, user, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			p, err := scanProg(rows)
			if err != nil {
				return err
			}
			out = append(out, p)
		}
		return rows.Err()
	})
	return out, err
}

// History — курсор (started_at, id) по убыванию. Индекс views_user_idx есть
// в каждой секции; при курсоре PostgreSQL отбрасывает секции новее него.
func (s Store) History(ctx context.Context, user uuid.UUID, before *usecase.Cursor, limit int) ([]core.View, error) {
	out := []core.View{}
	at, id := time.Now().Add(24*time.Hour), uuid.Max
	if before != nil {
		at, id = before.At, before.ID
	}
	err := s.as(ctx, user, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id, title_id, episode_id, via, started_at FROM views
			WHERE user_id=$1 AND (started_at, id) < ($2, $3)
			ORDER BY started_at DESC, id DESC LIMIT $4`, user, at, id, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var v core.View
			if err := rows.Scan(&v.ID, &v.TitleID, &v.EpisodeID, &v.Via, &v.StartedAt); err != nil {
				return err
			}
			out = append(out, v)
		}
		return rows.Err()
	})
	return out, err
}

func (s Store) Watchlist(ctx context.Context, user uuid.UUID) ([]usecase.WatchItem, error) {
	out := []usecase.WatchItem{}
	err := s.as(ctx, user, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT title_id, added_at FROM watchlist WHERE user_id=$1 ORDER BY added_at DESC LIMIT 200`, user)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var w usecase.WatchItem
			if err := rows.Scan(&w.TitleID, &w.AddedAt); err != nil {
				return err
			}
			out = append(out, w)
		}
		return rows.Err()
	})
	return out, err
}

func (s Store) AddWatch(ctx context.Context, user, title uuid.UUID) error {
	return s.as(ctx, user, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO watchlist(user_id, title_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`, user, title)
		return err
	})
}

func (s Store) RemoveWatch(ctx context.Context, user, title uuid.UUID) error {
	return s.as(ctx, user, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `DELETE FROM watchlist WHERE user_id=$1 AND title_id=$2`, user, title)
		return err
	})
}

func (s Store) InWatchlist(ctx context.Context, user, title uuid.UUID) (bool, error) {
	ok := false
	err := s.as(ctx, user, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM watchlist WHERE user_id=$1 AND title_id=$2)`, user, title).Scan(&ok)
	})
	return ok, err
}

// EnsurePartitions — заранее создаёт секции истории на 3 месяца вперёд.
func (s Store) EnsurePartitions(ctx context.Context) (int, error) {
	var n int
	err := s.DB.QueryRow(ctx, `SELECT ensure_view_partitions(3)`).Scan(&n)
	return n, err
}

// HistoryKeepMonths — сколько полных месяцев истории хранится.
const HistoryKeepMonths = 13

// DropOldPartitions отсоединяет и удаляет месячные секции истории старше
// keep месяцев (DETACH + DROP вместо DELETE).
func (s Store) DropOldPartitions(ctx context.Context, keep int) (int, error) {
	var n int
	err := s.DB.QueryRow(ctx, `SELECT drop_old_view_partitions($1)`, keep).Scan(&n)
	return n, err
}

// ApplyOrder — проекция события billing.orders.v1 (РХД ПР-5). Inbox и
// проекция в одной транзакции, контекст RLS — зритель из события. applied =
// false: событие уже обработано или устарело (ревизия не больше сохранённой).
func (s Store) ApplyOrder(ctx context.Context, env events.Envelope, o events.OrderSnapshot) (bool, error) {
	user, err := uuid.Parse(o.UserID)
	if err != nil {
		return false, err
	}
	var title *uuid.UUID
	if o.TitleID != "" {
		t, err := uuid.Parse(o.TitleID)
		if err != nil {
			return false, err
		}
		title = &t
	}
	applied := false
	err = s.as(ctx, user, func(tx pgx.Tx) error {
		dup, err := outbox.Seen(ctx, tx, env)
		if err != nil || dup {
			return err
		}
		tag, err := tx.Exec(ctx, `INSERT INTO order_feed(order_id, user_id, kind, title_id, status, revision, changed_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7)
			ON CONFLICT (order_id) DO UPDATE SET status=EXCLUDED.status, revision=EXCLUDED.revision, changed_at=EXCLUDED.changed_at
			WHERE order_feed.revision < EXCLUDED.revision`,
			o.OrderID, user, o.Kind, title, o.Status, env.Revision, o.ChangedAt)
		applied = err == nil && tag.RowsAffected() == 1
		return err
	})
	return applied, err
}

// OrderFeed — проекция заказов зрителя (для проверки и отладки).
func (s Store) OrderFeed(ctx context.Context, user uuid.UUID) ([]events.OrderSnapshot, error) {
	var out []events.OrderSnapshot
	err := s.as(ctx, user, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT order_id, user_id, kind, coalesce(title_id::text,''), status, changed_at
			FROM order_feed ORDER BY changed_at DESC`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var o events.OrderSnapshot
			var id, u uuid.UUID
			if err := rows.Scan(&id, &u, &o.Kind, &o.TitleID, &o.Status, &o.ChangedAt); err != nil {
				return err
			}
			o.OrderID, o.UserID = id.String(), u.String()
			out = append(out, o)
		}
		return rows.Err()
	})
	return out, err
}
