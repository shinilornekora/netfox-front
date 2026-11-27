package billing_test

// РХД ПР-4, требование к домашней системе: изменение бизнес-объекта, запись
// события и ответ команды — в одной транзакции; тесты на успешную фиксацию,
// повтор запроса, конфликт изменённого содержимого под прежним ключом, откат
// после записи события и изоляцию outbox политиками RLS.

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"plenka/billing/internal/adapter/pgsql"
	"plenka/billing/internal/core"
	"plenka/billing/internal/usecase"
	"plenka/contracts/events"
	"plenka/kit/bus"
	"plenka/kit/pgkit"
	"plenka/kit/pgtest"
	"plenka/kit/problem"
)

type outEvent struct {
	ID       string
	Status   string
	Revision int64
	Topic    string
}

// events читает outbox так, как это делает ретранслятор (plenka.relay = on).
func (e *env) events(t *testing.T, order uuid.UUID) []outEvent {
	t.Helper()
	var out []outEvent
	err := pgx.BeginFunc(context.Background(), e.owner, func(tx pgx.Tx) error {
		ctx := context.Background()
		if _, err := tx.Exec(ctx, `SELECT set_config('plenka.relay','on',true)`); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT body, topic FROM outbox WHERE msg_key = $1 ORDER BY seq`, order.String())
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var raw []byte
			var ev outEvent
			if err := rows.Scan(&raw, &ev.Topic); err != nil {
				return err
			}
			var envl events.Envelope
			if err := json.Unmarshal(raw, &envl); err != nil {
				return err
			}
			snap, err := bus.Decode[events.OrderSnapshot](envl)
			if err != nil {
				return err
			}
			ev.ID, ev.Status, ev.Revision = envl.ID, snap.Status, envl.Revision
			out = append(out, ev)
		}
		return rows.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func (e *env) outboxTotal(t *testing.T) int {
	t.Helper()
	var n int
	err := pgx.BeginFunc(context.Background(), e.owner, func(tx pgx.Tx) error {
		ctx := context.Background()
		if _, err := tx.Exec(ctx, `SELECT set_config('plenka.relay','on',true)`); err != nil {
			t.Fatal(err)
		}
		return tx.QueryRow(ctx, `SELECT count(*) FROM outbox`).Scan(&n)
	})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestOutboxCommittedWithOrder(t *testing.T) {
	e := setup(t)
	o, _, err := e.buy(uuid.New(), okCard, "k-ok-0001")
	if err != nil || o.Status != core.Completed {
		t.Fatalf("buy: %+v %v", o, err)
	}
	evs := e.events(t, o.ID)
	if len(evs) != 2 || evs[0].Status != "charging" || evs[1].Status != "completed" ||
		evs[0].Revision != 1 || evs[1].Revision != 2 || evs[0].Topic != "billing.orders.v1" {
		t.Fatalf("events: %+v", evs)
	}
	// Отказ банка: заказ в failed — и событие об этом.
	d, _, _ := e.buy(uuid.New(), declinedCard, "k-declined-1")
	if evs := e.events(t, d.ID); len(evs) != 2 || evs[1].Status != "failed" {
		t.Fatalf("declined events: %+v", evs)
	}
}

func TestOutboxRetrySameKeyAddsNothing(t *testing.T) {
	e := setup(t)
	u := uuid.New()
	o1, _, err := e.buy(u, okCard, "k-retry-001")
	if err != nil {
		t.Fatal(err)
	}
	before := e.outboxTotal(t)
	o2, replay, err := e.buy(u, okCard, "k-retry-001")
	if err != nil || !replay || o2.ID != o1.ID {
		t.Fatalf("replay: %+v %v %v", o2, replay, err)
	}
	if after := e.outboxTotal(t); after != before {
		t.Fatalf("retry added events: %d → %d", before, after)
	}
}

func TestOutboxConflictUnderSameKey(t *testing.T) {
	e := setup(t)
	u := uuid.New()
	if _, _, err := e.buy(u, okCard, "k-conflict-1"); err != nil {
		t.Fatal(err)
	}
	before := e.outboxTotal(t)
	// Тот же ключ, другое содержимое команды (подписка вместо покупки).
	_, _, err := e.subscribe(u, okCard, "k-conflict-1")
	if !problem.Is(err, "idempotency_mismatch") {
		t.Fatalf("want idempotency_mismatch, got %v", err)
	}
	if after := e.outboxTotal(t); after != before {
		t.Fatalf("conflict added events: %d → %d", before, after)
	}
}

// failAfter — хранилище, у которого шаг prepare падает уже ПОСЛЕ того, как
// заказ и событие записаны в транзакцию (например, сбой следующей команды).
type failAfter struct{ pgsql.Store }

var errBoom = errors.New("boom after outbox write")

func (f failAfter) Prepare(ctx context.Context, fn func(usecase.PrepTx) error) error {
	return f.Store.Prepare(ctx, func(tx usecase.PrepTx) error {
		if err := fn(tx); err != nil {
			return err
		}
		return errBoom
	})
}

func TestOutboxRolledBackWithOrder(t *testing.T) {
	e := setup(t)
	e.saga.Store = failAfter{e.store}
	u := uuid.New()
	if _, _, err := e.buy(u, okCard, "k-rollback-1"); !errors.Is(err, errBoom) {
		t.Fatalf("want boom, got %v", err)
	}
	var orders int
	if err := e.db.QueryRow(context.Background(), `SELECT count(*) FROM orders WHERE user_id=$1`, u).Scan(&orders); err != nil {
		t.Fatal(err)
	}
	t.Logf("сбой после записи события: заказов %d, событий %d", orders, e.outboxTotal(t))
	if orders != 0 || e.outboxTotal(t) != 0 {
		t.Fatalf("after rollback: orders=%d outbox=%d", orders, e.outboxTotal(t))
	}
}

// Два слоя защиты очереди событий.
//
//  1. Права: роль данных (под ней работают запросы) может только добавлять
//     события — читать, отмечать и удалять их не может, даже если выставит
//     plenka.relay = on. Проверяется, когда задан TEST_PG_APP_DSN.
//  2. RLS: даже владелец схемы без объявления ретранслятора не видит очередь,
//     не помечает события отправленными и не удаляет их. Это защита от
//     ошибки в коде владельца, а не от атаки: объявить себя ретранслятором
//     может любой код с правами владельца.
func TestOutboxIsolated(t *testing.T) {
	e := setup(t)
	if _, _, err := e.buy(uuid.New(), okCard, "k-rls-0001"); err != nil {
		t.Fatal(err)
	}
	total := e.outboxTotal(t)
	if total == 0 {
		t.Fatal("no events to protect")
	}
	ctx := context.Background()
	if pgtest.Split() {
		for _, q := range []string{
			`SELECT count(*) FROM outbox`,
			`UPDATE outbox SET sent_at = now()`,
			`DELETE FROM outbox`,
			`ALTER TABLE outbox DISABLE ROW LEVEL SECURITY`,
		} {
			err := pgx.BeginFunc(ctx, e.db, func(tx pgx.Tx) error {
				if _, err := tx.Exec(ctx, `SELECT set_config('plenka.relay','on',true)`); err != nil {
					return err
				}
				_, err := tx.Exec(ctx, q)
				return err
			})
			if c := pgkit.Code(err); c != "42501" {
				t.Fatalf("data role ran %q: code=%q err=%v", q, c, err)
			}
		}
	} else {
		t.Log("TEST_PG_APP_DSN не задан: проверяю только слой RLS")
	}
	var seen int
	if err := e.owner.QueryRow(ctx, `SELECT count(*) FROM outbox`).Scan(&seen); err != nil {
		t.Fatal(err)
	}
	if seen != 0 {
		t.Fatalf("code without relay context sees %d outbox rows", seen)
	}
	tag, err := e.owner.Exec(ctx, `UPDATE outbox SET sent_at = now()`)
	if err != nil || tag.RowsAffected() != 0 {
		t.Fatalf("marked events sent without relay context: %v %v", tag, err)
	}
	tag, err = e.owner.Exec(ctx, `DELETE FROM outbox`)
	if err != nil || tag.RowsAffected() != 0 {
		t.Fatalf("deleted events without relay context: %v %v", tag, err)
	}
	if e.outboxTotal(t) != total {
		t.Fatal("outbox changed")
	}
}

// ---- РХД ПР-4, вариант А: повтор команды и граница отката ----

// counts — «1/1/1» для заказа зрителя: сколько заказов, выданных покупок и
// событий в outbox.
func (e *env) counts(t *testing.T, user uuid.UUID) (orders, purchases, outboxRows int) {
	t.Helper()
	ctx := context.Background()
	if err := e.owner.QueryRow(ctx, `SELECT (SELECT count(*) FROM orders WHERE user_id=$1),
		(SELECT count(*) FROM purchases WHERE user_id=$1)`, user).Scan(&orders, &purchases); err != nil {
		t.Fatal(err)
	}
	return orders, purchases, e.outboxTotal(t)
}

func eventIDs(evs []outEvent) string {
	ids := make([]string, len(evs))
	for i, ev := range evs {
		ids[i] = ev.ID
	}
	return strings.Join(ids, ",")
}

// А-1. Восемь одновременных запросов с одним ключом, а после них — ещё пять
// последовательных повторов того же запроса (ключ тот же, в цикле новый не
// создаётся). Каждый повтор возвращает прежний ответ, идентификаторы событий
// не меняются, а в базе остаётся один заказ, одна покупка и одна пара событий.
func TestOutboxConcurrentDuplicateThenReplays(t *testing.T) {
	e := setup(t)
	u, key := uuid.New(), "k-dup-concurrent"
	var wg sync.WaitGroup
	ids := make([]uuid.UUID, 8)
	errs := make([]error, 8)
	for i := range ids {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			o, _, err := e.buy(u, okCard, key)
			ids[i], errs[i] = o.ID, err
		}(i)
	}
	wg.Wait()
	var order uuid.UUID
	for i, err := range errs {
		// Параллельный повтор, заставший заказ в работе, получает «подождите» —
		// это отказ без побочных эффектов; все успешные ответы называют один заказ.
		if err != nil {
			if !problem.Is(err, "order_in_progress") {
				t.Fatalf("goroutine %d: %v", i, err)
			}
			continue
		}
		if order == uuid.Nil {
			order = ids[i]
		}
		if ids[i] != order {
			t.Fatalf("concurrent duplicates returned different orders: %v", ids)
		}
	}
	if order == uuid.Nil {
		t.Fatal("no successful response among concurrent duplicates")
	}
	first := e.events(t, order)
	if len(first) != 2 {
		t.Fatalf("events after concurrent duplicates: %+v", first)
	}
	for i := 1; i <= 5; i++ {
		o, replay, err := e.buy(u, okCard, key)
		if err != nil || !replay || o.ID != order || o.Status != core.Completed {
			t.Fatalf("replay %d: %+v replay=%v err=%v", i, o, replay, err)
		}
		if got := eventIDs(e.events(t, order)); got != eventIDs(first) {
			t.Fatalf("replay %d changed event ids: %s → %s", i, eventIDs(first), got)
		}
		if orders, purchases, rows := e.counts(t, u); orders != 1 || purchases != 1 || rows != 2 {
			t.Fatalf("replay %d: orders=%d purchases=%d outbox=%d, want 1/1/2", i, orders, purchases, rows)
		}
	}
	t.Logf("заказ %s: 8 одновременных запросов и 5 повторов → 1 заказ, 1 покупка, события %s", order, eventIDs(first))
}

// А-2. Прежний ключ, но другая цель команды — другой фильм. Команда
// отклоняется как конфликт идемпотентности, и отдельным запросом проверено,
// что заказа и покупки второго фильма в базе не появилось.
func TestOutboxSameKeyOtherTitleCreatesNothing(t *testing.T) {
	e := setup(t)
	other := uuid.New()
	e.title(t, other, "film", "published", true, 19900, 1)
	u, key := uuid.New(), "k-same-key-01"
	if _, _, err := e.buy(u, okCard, key); err != nil {
		t.Fatal(err)
	}
	before := e.outboxTotal(t)
	_, _, err := e.saga.Checkout(viewer(u), usecase.CheckoutInput{Kind: core.KindPurchase, TitleID: other, Card: okCard, Key: key})
	if !problem.Is(err, "idempotency_mismatch") {
		t.Fatalf("want idempotency_mismatch, got %v", err)
	}
	var orders, purchases int
	if err := e.owner.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM orders WHERE title_id=$1),
		(SELECT count(*) FROM purchases WHERE title_id=$1)`, other).Scan(&orders, &purchases); err != nil {
		t.Fatal(err)
	}
	t.Logf("тот же ключ, другой фильм → %v", err)
	t.Logf("заказов второго фильма: %d, покупок: %d, событий в outbox: %d → %d", orders, purchases, before, e.outboxTotal(t))
	if orders != 0 || purchases != 0 || e.outboxTotal(t) != before {
		t.Fatalf("conflicting command left traces: orders=%d purchases=%d outbox %d→%d", orders, purchases, before, e.outboxTotal(t))
	}
}

// А-3. Управляемый сбой в момент записи события: строка заказа уже вставлена
// в транзакцию, а добавление события в outbox отказывает (триггер на таблице
// outbox — так сбой происходит ровно в этой точке, без правок кода сервиса).
// Итог 0/0/0 — тот же, что при сбое ПОСЛЕ записи события
// (TestOutboxRolledBackWithOrder): обе точки лежат внутри одной транзакции,
// а до её фиксации не существует ни заказа, ни события.
func TestOutboxFailureBeforeEventAppend(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	for _, q := range []string{
		`CREATE FUNCTION outbox_down() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'outbox is down'; END $$`,
		`CREATE TRIGGER outbox_down BEFORE INSERT ON outbox FOR EACH ROW EXECUTE FUNCTION outbox_down()`,
	} {
		if _, err := e.owner.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	u := uuid.New()
	_, _, err := e.buy(u, okCard, "k-before-evt")
	if err == nil || !strings.Contains(err.Error(), "outbox is down") {
		t.Fatalf("want the injected failure at event append, got %v", err)
	}
	t.Logf("сбой при записи события: %v", err)
	orders, purchases, rows := e.counts(t, u)
	t.Logf("после сбоя: заказов %d, покупок %d, событий %d", orders, purchases, rows)
	if orders != 0 || purchases != 0 || rows != 0 {
		t.Fatalf("after failure before event append: %d/%d/%d, want 0/0/0", orders, purchases, rows)
	}
	// Сбой устранён — тот же запрос с тем же ключом проходит как новый.
	if _, err := e.owner.Exec(ctx, `DROP TRIGGER outbox_down ON outbox`); err != nil {
		t.Fatal(err)
	}
	o, replay, err := e.buy(u, okCard, "k-before-evt")
	t.Logf("сбой устранён, тот же ключ: заказ %s, статус %s, повтор=%v", o.ID, o.Status, replay)
	if err != nil || replay || o.Status != core.Completed {
		t.Fatalf("retry after the failure: %+v replay=%v err=%v", o, replay, err)
	}
}
