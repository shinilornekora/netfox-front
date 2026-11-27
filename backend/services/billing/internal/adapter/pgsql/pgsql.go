// Package pgsql — хранилище billing.
package pgsql

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"plenka/billing/internal/core"
	"plenka/billing/internal/usecase"
	"plenka/contracts/events"
	"plenka/kit/outbox"
	"plenka/kit/pgkit"
	"plenka/kit/problem"
)

type Store struct {
	DB *pgxpool.Pool
	// WeakPrepare — только для эксперимента РХД ПР-3 (тест
	// TestPurchaseWriteSkewExperiment): шаг prepare идёт в READ COMMITTED,
	// чтобы показать, что без SERIALIZABLE одна покупка списывается дважды.
	// В сервисе всегда false.
	WeakPrepare bool
	// BeforeInsert — точка синхронизации для того же эксперимента: вызывается
	// после всех проверок, перед вставкой заказа.
	BeforeInsert func()
}

var (
	_ usecase.Store  = Store{}
	_ usecase.Ledger = Store{}
)

const orderCols = `id, user_id, kind, coalesce(plan_id,''), title_id, title_name, amount_kopecks, status, failure,
	card_last4, idem_key, request_hash, created_at, updated_at`

func scanOrder(row pgx.Row) (core.Order, error) {
	var o core.Order
	var kind, status string
	err := row.Scan(&o.ID, &o.UserID, &kind, &o.PlanID, &o.TitleID, &o.TitleName, &o.Amount, &status, &o.Failure,
		&o.CardLast4, &o.IdemKey, &o.RequestHash, &o.CreatedAt, &o.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return o, problem.NotFound("Заказ")
	}
	o.Kind, o.Status = core.Kind(kind), core.Status(status)
	return o, err
}

func collect(rows pgx.Rows, err error) ([]core.Order, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []core.Order{}
	for rows.Next() {
		o, err := scanOrder(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// ---------- шаг prepare: SERIALIZABLE ----------

func (s Store) Prepare(ctx context.Context, fn func(usecase.PrepTx) error) error {
	body := func(tx pgx.Tx) error { return fn(prep{tx, s.BeforeInsert}) }
	if s.WeakPrepare {
		return pgkit.ReadCommitted(ctx, s.DB, body)
	}
	// Два одновременных запроса с одним ключом идемпотентности оба не видят
	// заказа и оба вставляют. PostgreSQL сообщает проигравшему не об ошибке
	// сериализации, а о нарушении уникальности ключа (23505). Это не отказ
	// команды: первый запрос её уже выполняет. Повторяем шаг — теперь он
	// найдёт заказ по ключу и вернёт его как повтор.
	for attempt := 1; ; attempt++ {
		err := pgkit.Serializable(ctx, s.DB, body)
		var pe *pgconn.PgError
		if attempt < 4 && errors.As(err, &pe) && pe.Code == "23505" && pe.ConstraintName == "orders_idem_uq" {
			continue
		}
		return err
	}
}

type prep struct {
	tx     pgx.Tx
	before func()
}

func (p prep) OrderByKey(ctx context.Context, user uuid.UUID, key string) (*core.Order, error) {
	o, err := scanOrder(p.tx.QueryRow(ctx, `SELECT `+orderCols+` FROM orders WHERE user_id=$1 AND idem_key=$2`, user, key))
	if problem.Is(err, "not_found") {
		return nil, nil
	}
	return &o, err
}

func (p prep) Plan(ctx context.Context, id string) (core.Plan, error) {
	var pl core.Plan
	err := p.tx.QueryRow(ctx, `SELECT id, name, period_days, price_kopecks FROM plans WHERE id=$1 AND active`, id).
		Scan(&pl.ID, &pl.Name, &pl.PeriodDays, &pl.PriceKopecks)
	if errors.Is(err, pgx.ErrNoRows) {
		return pl, problem.NotFound("Тариф")
	}
	return pl, err
}

func title(ctx context.Context, q pgkit.Q, id uuid.UUID) (core.TitleOffer, bool, error) {
	var t core.TitleOffer
	err := q.QueryRow(ctx, `SELECT id, name, kind, status, in_subscription, price_kopecks FROM title_replica WHERE id=$1`, id).
		Scan(&t.ID, &t.Name, &t.Kind, &t.Status, &t.InSubscription, &t.PriceKopecks)
	if errors.Is(err, pgx.ErrNoRows) {
		return core.TitleOffer{ID: id}, false, nil
	}
	return t, err == nil, err
}

func (p prep) Title(ctx context.Context, id uuid.UUID) (core.TitleOffer, bool, error) {
	return title(ctx, p.tx, id)
}

func (p prep) HasPurchase(ctx context.Context, user, t uuid.UUID) (bool, error) {
	var ok bool
	err := p.tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM purchases WHERE user_id=$1 AND title_id=$2)`, user, t).Scan(&ok)
	return ok, err
}

func activeSub(ctx context.Context, q pgkit.Q, user uuid.UUID, now time.Time) (*core.Subscription, error) {
	var s core.Subscription
	err := q.QueryRow(ctx, `SELECT s.id, s.user_id, s.plan_id, p.name, s.started_at, s.current_period_end, s.cancel_at_period_end
		FROM subscriptions s JOIN plans p ON p.id = s.plan_id
		WHERE s.user_id=$1 AND s.status='active' AND s.current_period_end > $2`, user, now).
		Scan(&s.ID, &s.UserID, &s.PlanID, &s.PlanName, &s.StartedAt, &s.CurrentPeriodEnd, &s.CancelAtPeriodEnd)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &s, err
}

func (p prep) ActiveSubscription(ctx context.Context, user uuid.UUID, now time.Time) (*core.Subscription, error) {
	return activeSub(ctx, p.tx, user, now)
}

func (p prep) InFlight(ctx context.Context, user uuid.UUID, target string) (bool, error) {
	var ok bool
	err := p.tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM orders WHERE user_id=$1 AND status IN ('charging','refunding')
		AND (CASE WHEN kind='subscription' THEN 'subscription' ELSE 'title:' || title_id::text END) = $2)`, user, target).Scan(&ok)
	return ok, err
}

func (p prep) InsertOrder(ctx context.Context, o core.Order) error {
	if p.before != nil {
		p.before()
	}
	var plan *string
	if o.PlanID != "" {
		plan = &o.PlanID
	}
	_, err := p.tx.Exec(ctx, `INSERT INTO orders(id, user_id, kind, plan_id, title_id, title_name, amount_kopecks, status, card_last4,
		idem_key, request_hash, created_at, updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		o.ID, o.UserID, string(o.Kind), plan, o.TitleID, o.TitleName, o.Amount, string(o.Status), o.CardLast4,
		o.IdemKey, o.RequestHash, o.CreatedAt, o.UpdatedAt)
	if err != nil {
		return err
	}
	// Заказ (бизнес-объект и сохранённый ответ команды по ключу
	// идемпотентности) и событие — в одной транзакции (РХД ПР-4).
	return orderEvent(ctx, p.tx, o)
}

// orderEvent кладёт событие billing.orders.v1 в outbox текущей транзакции.
func orderEvent(ctx context.Context, tx pgx.Tx, o core.Order) error {
	snap := events.OrderSnapshot{OrderID: o.ID.String(), UserID: o.UserID.String(), Kind: string(o.Kind),
		PlanID: o.PlanID, AmountKop: o.Amount, Status: string(o.Status), ChangedAt: o.UpdatedAt.UTC()}
	if o.TitleID != nil {
		snap.TitleID = o.TitleID.String()
	}
	return outbox.Add(ctx, tx, events.TopicOrders, "billing", events.OrderStatusChanged, o.ID.String(),
		events.OrderRevision(string(o.Status)), o.IdemKey, snap)
}

// ---------- шаг fulfil: SERIALIZABLE ----------

func cannot(msg string) error { return problem.Conflict("cannot_fulfil", msg) }

func (s Store) Fulfil(ctx context.Context, id uuid.UUID, now time.Time) (core.Order, error) {
	var out core.Order
	err := pgkit.Serializable(ctx, s.DB, func(tx pgx.Tx) error {
		o, err := scanOrder(tx.QueryRow(ctx, `SELECT `+orderCols+` FROM orders WHERE id=$1 FOR UPDATE`, id))
		if err != nil {
			return err
		}
		if o.Status == core.Completed { // повторный fulfil (recovery) — идемпотентно
			out = o
			return nil
		}
		if o.Status != core.Charging {
			return problem.Conflict("bad_state", "Заказ уже в статусе "+string(o.Status))
		}
		switch o.Kind {
		case core.KindPurchase:
			tag, err := tx.Exec(ctx, `INSERT INTO purchases(user_id, title_id, order_id, purchased_at) VALUES ($1,$2,$3,$4)
				ON CONFLICT DO NOTHING`, o.UserID, o.TitleID, o.ID, now)
			if err != nil {
				return err
			}
			if tag.RowsAffected() == 0 {
				return cannot("Фильм уже был куплен другим заказом")
			}
		case core.KindSubscription:
			if cur, err := activeSub(ctx, tx, o.UserID, now); err != nil {
				return err
			} else if cur != nil {
				return cannot("Подписка уже действует")
			}
			var days int
			if err := tx.QueryRow(ctx, `SELECT period_days FROM plans WHERE id=$1`, o.PlanID).Scan(&days); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE subscriptions SET status='expired' WHERE user_id=$1 AND status='active'`, o.UserID); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO subscriptions(id, user_id, plan_id, order_id, status, started_at, current_period_end)
				VALUES ($1,$2,$3,$4,'active',$5,$6)`, uuid.New(), o.UserID, o.PlanID, o.ID, now, now.AddDate(0, 0, days)); err != nil {
				if pgkit.Constraint(err) == "subscriptions_one_active" {
					return cannot("Подписка уже действует")
				}
				return err
			}
		}
		out, err = scanOrder(tx.QueryRow(ctx, `UPDATE orders SET status='completed', updated_at=$2 WHERE id=$1 RETURNING `+orderCols, id, now))
		if err != nil {
			return err
		}
		return orderEvent(ctx, tx, out)
	})
	return out, err
}

// Revoke — первый шаг возврата выданного заказа: в одной SERIALIZABLE-
// транзакции забрать право (покупку или подписку), перевести заказ
// completed → refunding и записать событие. Деньги возвращает оркестратор
// уже после коммита; если он упадёт, recovery увидит refunding и повторит.
func (s Store) Revoke(ctx context.Context, id uuid.UUID, reason string, now time.Time) (core.Order, error) {
	var out core.Order
	err := pgkit.Serializable(ctx, s.DB, func(tx pgx.Tx) error {
		o, err := scanOrder(tx.QueryRow(ctx, `SELECT `+orderCols+` FROM orders WHERE id=$1 FOR UPDATE`, id))
		if err != nil {
			return err
		}
		switch o.Status {
		case core.Refunding, core.Refunded: // повтор — идемпотентно
			out = o
			return nil
		case core.Completed:
		default:
			return problem.Conflict("bad_state", "Вернуть можно только оплаченный заказ, сейчас он в статусе "+string(o.Status))
		}
		switch o.Kind {
		case core.KindPurchase:
			_, err = tx.Exec(ctx, `DELETE FROM purchases WHERE order_id=$1`, o.ID)
		case core.KindSubscription:
			_, err = tx.Exec(ctx, `UPDATE subscriptions SET status='expired', current_period_end=least(current_period_end, $2)
				WHERE order_id=$1 AND status='active'`, o.ID, now)
		}
		if err != nil {
			return err
		}
		out, err = scanOrder(tx.QueryRow(ctx, `UPDATE orders SET status='refunding', failure=$2, updated_at=$3
			WHERE id=$1 RETURNING `+orderCols, id, reason, now))
		if err != nil {
			return err
		}
		return orderEvent(ctx, tx, out)
	})
	return out, err
}

func (s Store) Transition(ctx context.Context, id uuid.UUID, from []core.Status, to core.Status, failure string) (core.Order, error) {
	fs := make([]string, len(from))
	for i, f := range from {
		fs[i] = string(f)
	}
	var o core.Order
	err := pgkit.ReadCommitted(ctx, s.DB, func(tx pgx.Tx) error {
		var err error
		o, err = scanOrder(tx.QueryRow(ctx, `UPDATE orders SET status=$3, failure=CASE WHEN $4 = '' THEN failure ELSE $4 END, updated_at=now()
			WHERE id=$1 AND status = ANY($2) RETURNING `+orderCols, id, fs, string(to), failure))
		if err != nil {
			return err
		}
		return orderEvent(ctx, tx, o) // смена статуса и событие — атомарно
	})
	if problem.Is(err, "not_found") {
		cur, gerr := s.Order(ctx, id)
		if gerr != nil {
			return cur, gerr
		}
		if cur.Status == to {
			return cur, nil // уже переведён (повтор)
		}
		return cur, problem.Conflict("bad_state", "Заказ уже в статусе "+string(cur.Status))
	}
	return o, err
}

func (s Store) Order(ctx context.Context, id uuid.UUID) (core.Order, error) {
	return scanOrder(s.DB.QueryRow(ctx, `SELECT `+orderCols+` FROM orders WHERE id=$1`, id))
}

func (s Store) Stuck(ctx context.Context, older time.Time) ([]core.Order, error) {
	return collect(s.DB.Query(ctx, `SELECT `+orderCols+` FROM orders WHERE status IN ('charging','refunding') AND updated_at < $1
		ORDER BY updated_at LIMIT 100`, older))
}

// ---------- кабинет и доступ ----------

func (s Store) Plans(ctx context.Context) ([]core.Plan, error) {
	rows, err := s.DB.Query(ctx, `SELECT id, name, period_days, price_kopecks FROM plans WHERE active ORDER BY period_days`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (core.Plan, error) {
		var p core.Plan
		return p, r.Scan(&p.ID, &p.Name, &p.PeriodDays, &p.PriceKopecks)
	})
}

func (s Store) Subscription(ctx context.Context, user uuid.UUID, now time.Time) (*core.Subscription, error) {
	return activeSub(ctx, s.DB, user, now)
}

func (s Store) Purchases(ctx context.Context, user uuid.UUID) ([]core.Purchase, error) {
	rows, err := s.DB.Query(ctx, `SELECT p.title_id, o.title_name, p.purchased_at, o.amount_kopecks
		FROM purchases p JOIN orders o ON o.id = p.order_id WHERE p.user_id=$1 ORDER BY p.purchased_at DESC`, user)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (core.Purchase, error) {
		var p core.Purchase
		return p, r.Scan(&p.TitleID, &p.TitleName, &p.PurchasedAt, &p.Amount)
	})
}

func (s Store) Orders(ctx context.Context, user uuid.UUID, limit int) ([]core.Order, error) {
	return collect(s.DB.Query(ctx, `SELECT `+orderCols+` FROM orders WHERE user_id=$1 ORDER BY created_at DESC LIMIT $2`, user, limit))
}

func (s Store) SetCancelAtPeriodEnd(ctx context.Context, user uuid.UUID, now time.Time, cancel bool) (*core.Subscription, error) {
	if _, err := s.DB.Exec(ctx, `UPDATE subscriptions SET cancel_at_period_end=$3
		WHERE user_id=$1 AND status='active' AND current_period_end > $2`, user, now, cancel); err != nil {
		return nil, err
	}
	return activeSub(ctx, s.DB, user, now)
}

func (s Store) AccessFacts(ctx context.Context, user, t uuid.UUID, now time.Time) (core.TitleOffer, bool, bool, *core.Subscription, error) {
	offer, known, err := title(ctx, s.DB, t)
	if err != nil {
		return offer, false, false, nil, err
	}
	var owned bool
	if err := s.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM purchases WHERE user_id=$1 AND title_id=$2)`, user, t).Scan(&owned); err != nil {
		return offer, known, false, nil, err
	}
	sub, err := activeSub(ctx, s.DB, user, now)
	return offer, known, owned, sub, err
}

// ApplyTitle — проекция: inbox + обновление, только если ревизия новее.
func (s Store) ApplyTitle(ctx context.Context, env events.Envelope, t events.TitleSnapshot) error {
	return pgkit.ReadCommitted(ctx, s.DB, func(tx pgx.Tx) error {
		dup, err := outbox.Seen(ctx, tx, env)
		if err != nil || dup {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO title_replica(id, name, kind, status, in_subscription, price_kopecks, revision)
			VALUES ($1,$2,$3,$4,$5,$6,$7)
			ON CONFLICT (id) DO UPDATE SET name=EXCLUDED.name, kind=EXCLUDED.kind, status=EXCLUDED.status,
				in_subscription=EXCLUDED.in_subscription, price_kopecks=EXCLUDED.price_kopecks, revision=EXCLUDED.revision
			WHERE title_replica.revision < EXCLUDED.revision`,
			t.ID, t.Name, t.Kind, t.Status, t.InSubscription, t.PriceKopecks, t.Revision)
		return err
	})
}

func (s Store) AdminOrders(ctx context.Context, status string, before *time.Time, limit int) ([]core.Order, error) {
	b := time.Now().Add(time.Hour)
	if before != nil {
		b = *before
	}
	return collect(s.DB.Query(ctx, `SELECT `+orderCols+` FROM orders WHERE ($1 = '' OR status = $1) AND created_at < $2
		ORDER BY created_at DESC, id DESC LIMIT $3`, status, b, limit))
}

func (s Store) Revenue(ctx context.Context, days int, now time.Time) (usecase.Revenue, error) {
	var r usecase.Revenue
	since := now.AddDate(0, 0, -days)
	err := s.DB.QueryRow(ctx, `SELECT
			coalesce(sum(amount_kopecks) FILTER (WHERE status='completed'), 0),
			count(*) FILTER (WHERE status='completed'),
			count(*) FILTER (WHERE status='refunded'),
			count(*) FILTER (WHERE status='failed'),
			(SELECT count(*) FROM subscriptions WHERE status='active' AND current_period_end > $2)
		FROM orders WHERE created_at >= $1`, since, now).
		Scan(&r.TotalKopecks, &r.CompletedOrders, &r.RefundedOrders, &r.FailedOrders, &r.ActiveSubscriptions)
	if err != nil {
		return r, err
	}
	rows, err := s.DB.Query(ctx, `SELECT to_char(d, 'YYYY-MM-DD'),
			coalesce(sum(o.amount_kopecks), 0), count(o.id)
		FROM generate_series($1::date, $2::date, interval '1 day') d
		LEFT JOIN orders o ON o.status='completed' AND o.created_at >= d AND o.created_at < d + interval '1 day'
		GROUP BY d ORDER BY d`, since, now)
	if err != nil {
		return r, err
	}
	r.Days, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (usecase.DayRevenue, error) {
		var d usecase.DayRevenue
		return d, row.Scan(&d.Day, &d.Kopecks, &d.Orders)
	})
	return r, err
}
