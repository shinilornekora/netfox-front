// Package outbox — исходящие события с арендой (РХД-4/5).
//
// Запись: событие кладётся в таблицу outbox той же транзакцией, что и
// изменение, и триггер шлёт NOTIFY — ретранслятор просыпается сразу, а не
// по таймеру.
//
// Доставка: ретранслятор берёт пачку записей в аренду (lease_owner,
// lease_until) короткой транзакцией и публикует их уже ВНЕ транзакции. Если
// процесс упадёт посреди публикации, аренда истечёт и записи заберёт другой
// экземпляр. Транзакция не держит блокировки, пока мы ждём Kafka.
package outbox

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"plenka/contracts/events"
)

// Schema — DDL, который сервис включает в свою миграцию.
const Schema = `
CREATE TABLE outbox (
    seq          bigserial PRIMARY KEY,
    event_id     uuid NOT NULL UNIQUE,
    topic        text NOT NULL,
    msg_key      text NOT NULL,
    body         jsonb NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    lease_owner  text,
    lease_until  timestamptz,
    attempts     int NOT NULL DEFAULT 0,
    last_error   text,
    sent_at      timestamptz
);
CREATE INDEX outbox_pending ON outbox (seq) WHERE sent_at IS NULL;
CREATE FUNCTION outbox_notify() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN PERFORM pg_notify('outbox', current_schema()); RETURN NULL; END $$;
CREATE TRIGGER outbox_notify AFTER INSERT ON outbox FOR EACH STATEMENT EXECUTE FUNCTION outbox_notify();
`

// Add записывает событие в рамках транзакции tx.
func Add(ctx context.Context, tx pgx.Tx, topic, source, typ, aggregate string, revision int64, correlation string, data any) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	env := events.Envelope{
		ID: uuid.NewString(), Type: typ, Version: 1, Aggregate: aggregate, Revision: revision,
		OccurredAt: time.Now().UTC(), Source: source, Correlation: correlation, Data: raw,
	}
	body, err := json.Marshal(env)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO outbox(event_id, topic, msg_key, body) VALUES ($1,$2,$3,$4)`, env.ID, topic, aggregate, body)
	return err
}

// Message — то, что ретранслятор отдаёт брокеру.
type Message struct {
	Topic, Key string
	Value      []byte
}

type Publisher interface {
	Publish(ctx context.Context, msgs []Message) error
}

type Relay struct {
	Pool     *pgxpool.Pool
	Pub      Publisher
	Log      *slog.Logger
	Owner    string        // идентификатор экземпляра
	Lease    time.Duration // срок аренды
	Batch    int
	Fallback time.Duration // опрос на случай пропущенного NOTIFY
}

func (r *Relay) defaults() {
	if r.Owner == "" {
		h, _ := os.Hostname()
		r.Owner = fmt.Sprintf("%s-%s", h, uuid.NewString()[:8])
	}
	if r.Lease == 0 {
		r.Lease = 30 * time.Second
	}
	if r.Batch == 0 {
		r.Batch = 100
	}
	if r.Fallback == 0 {
		r.Fallback = 5 * time.Second
	}
}

// Run слушает NOTIFY и доставляет события, пока не отменён ctx.
func (r *Relay) Run(ctx context.Context) {
	r.defaults()
	wake := make(chan struct{}, 1)
	go r.listen(ctx, wake)
	t := time.NewTicker(r.Fallback)
	defer t.Stop()
	for {
		for {
			n, err := r.Once(ctx)
			if err != nil {
				if ctx.Err() == nil {
					r.Log.Warn("outbox relay", "err", err)
				}
				break
			}
			if n < r.Batch {
				break
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-wake:
		case <-t.C:
		}
	}
}

func (r *Relay) listen(ctx context.Context, wake chan<- struct{}) {
	for ctx.Err() == nil {
		conn, err := r.Pool.Acquire(ctx)
		if err != nil {
			time.Sleep(time.Second)
			continue
		}
		if _, err := conn.Exec(ctx, `LISTEN outbox`); err == nil {
			for {
				if _, err := conn.Conn().WaitForNotification(ctx); err != nil {
					break
				}
				select {
				case wake <- struct{}{}:
				default:
				}
			}
		}
		conn.Release()
	}
}

// Once: взять пачку в аренду → опубликовать → отметить отправленными.
//
// Каждая операция с таблицей — отдельная короткая транзакция, в которой
// ретранслятор объявляет себя (plenka.relay = on): в billing таблица outbox
// закрыта RLS, и без этого он не увидит ни одной записи.
func (r *Relay) Once(ctx context.Context) (int, error) {
	r.defaults()
	var seqs []int64
	var msgs []Message
	err := r.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
		UPDATE outbox SET lease_owner = $1, lease_until = now() + $2::interval, attempts = attempts + 1
		WHERE seq IN (
			SELECT seq FROM outbox
			WHERE sent_at IS NULL AND (lease_until IS NULL OR lease_until < now())
			ORDER BY seq LIMIT $3
			FOR UPDATE SKIP LOCKED)
		RETURNING seq, topic, msg_key, body`, r.Owner, fmt.Sprintf("%d milliseconds", r.Lease.Milliseconds()), r.Batch)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var s int64
			var m Message
			if err := rows.Scan(&s, &m.Topic, &m.Key, &m.Value); err != nil {
				return err
			}
			seqs = append(seqs, s)
			msgs = append(msgs, m)
		}
		return rows.Err()
	})
	if err != nil || len(msgs) == 0 {
		return 0, err
	}
	// RETURNING не гарантирует порядок — восстанавливаем порядок seq,
	// чтобы события одного агрегата ушли в брокер по очереди.
	sortBySeq(seqs, msgs)

	pctx, cancel := context.WithTimeout(ctx, r.Lease/2)
	perr := r.Pub.Publish(pctx, msgs)
	cancel()
	if perr != nil {
		// Аренду не снимаем: записи вернутся в работу, когда она истечёт.
		_ = r.tx(ctx, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `UPDATE outbox SET last_error=$2 WHERE seq = ANY($1)`, seqs, perr.Error())
			return err
		})
		return 0, perr
	}
	// Отмечаем только то, что всё ещё наше: если аренда успела истечь и
	// записи забрал другой экземпляр, второй раз их не отметим (дубль
	// доставки возможен — потребители идемпотентны).
	err = r.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE outbox SET sent_at = now(), lease_owner = NULL, lease_until = NULL
		WHERE seq = ANY($1) AND lease_owner = $2`, seqs, r.Owner)
		return err
	})
	return len(seqs), err
}

func (r *Relay) tx(ctx context.Context, fn func(pgx.Tx) error) error {
	tctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return pgx.BeginFunc(tctx, r.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(tctx, `SELECT set_config('plenka.relay', 'on', true)`); err != nil {
			return err
		}
		return fn(tx)
	})
}

func sortBySeq(seqs []int64, msgs []Message) {
	for i := 1; i < len(seqs); i++ {
		for j := i; j > 0 && seqs[j] < seqs[j-1]; j-- {
			seqs[j], seqs[j-1] = seqs[j-1], seqs[j]
			msgs[j], msgs[j-1] = msgs[j-1], msgs[j]
		}
	}
}

// InboxSchema — таблица обработанных событий потребителя (РХД-5).
const InboxSchema = `
CREATE TABLE inbox (
    event_id     uuid PRIMARY KEY,
    event_type   text NOT NULL,
    handled_at   timestamptz NOT NULL DEFAULT now()
);`

// Seen отмечает событие обработанным; false — уже было.
func Seen(ctx context.Context, tx pgx.Tx, env events.Envelope) (bool, error) {
	tag, err := tx.Exec(ctx, `INSERT INTO inbox(event_id, event_type) VALUES ($1,$2) ON CONFLICT DO NOTHING`, env.ID, env.Type)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 0, nil
}
