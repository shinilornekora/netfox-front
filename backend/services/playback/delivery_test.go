package playback_test

// РХД ПР-5, сквозной домашний проект: надёжный обработчик события
// billing.orders.v1, формирующий локальную проекцию order_feed. Сценарии:
// повторная доставка, падение после коммита в базу (до фиксации смещения),
// устаревшее событие, изоляция зрителей RLS. Poison pill → DLQ проверяется
// на настоящей Kafka в kit/bus (TestPoisonPillGoesToDLQ).

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"plenka/contracts/events"
	"plenka/kit/bus"
	"plenka/kit/pgkit"
	"plenka/playback/internal/adapter/pgsql"
)

// forgetful — fakeAccess, который запоминает сбросы кэша.
type forgetful struct {
	fakeAccess
	mu     sync.Mutex
	forgot []string
}

func (f *forgetful) Forget(user, title uuid.UUID) {
	f.mu.Lock()
	f.forgot = append(f.forgot, user.String()+"/"+title.String())
	f.mu.Unlock()
}
func (f *forgetful) ForgetUser(user uuid.UUID) {
	f.mu.Lock()
	f.forgot = append(f.forgot, user.String()+"/*")
	f.mu.Unlock()
}

func orderEvent(t *testing.T, id string, order, user, title uuid.UUID, status string) events.Envelope {
	t.Helper()
	snap := events.OrderSnapshot{OrderID: order.String(), UserID: user.String(), Kind: "purchase",
		TitleID: title.String(), AmountKop: 29900, Status: status, ChangedAt: time.Now().UTC()}
	data, _ := json.Marshal(snap)
	return events.Envelope{ID: id, Type: events.OrderStatusChanged, Aggregate: order.String(),
		Revision: events.OrderRevision(status), Source: "billing", Data: data}
}

func (e *env) feed(t *testing.T, user uuid.UUID) []events.OrderSnapshot {
	t.Helper()
	out, err := pgsql.Store{DB: e.db}.OrderFeed(context.Background(), user)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestOrderFeedRedelivery(t *testing.T) {
	e := setup(t)
	acc := &forgetful{fakeAccess: e.uc.Access.(fakeAccess)}
	e.uc.Access = acc
	u, order := uuid.New(), uuid.New()
	if err := e.uc.HandleOrders(context.Background(), orderEvent(t, uuid.NewString(), order, u, e.film, "completed")); err != nil {
		t.Fatal(err)
	}
	if len(acc.forgot) != 0 {
		t.Fatalf("a purchase must not wipe cached grants: %v", acc.forgot)
	}
	ev := orderEvent(t, uuid.NewString(), order, u, e.film, "refunded")
	for i := 0; i < 3; i++ { // брокер доставил одно и то же три раза
		if err := e.uc.HandleOrders(context.Background(), ev); err != nil {
			t.Fatal(err)
		}
	}
	if f := e.feed(t, u); len(f) != 1 || f[0].Status != "refunded" {
		t.Fatalf("feed: %+v", f)
	}
	if len(acc.forgot) != 1 || acc.forgot[0] != u.String()+"/"+e.film.String() {
		t.Fatalf("refund must reset the cached grant exactly once, got %v", acc.forgot)
	}
}

// Падение после коммита: обработчик зафиксировал проекцию и inbox, а процесс
// умер до фиксации смещения в Kafka. После перезапуска брокер доставит то же
// сообщение снова — и ещё позже придёт событие, обогнанное более новым.
func TestOrderFeedCrashAfterCommit(t *testing.T) {
	e := setup(t)
	u, order := uuid.New(), uuid.New()
	completed := orderEvent(t, uuid.NewString(), order, u, e.film, "completed")
	ctx := context.Background()
	if err := e.uc.HandleOrders(ctx, completed); err != nil { // коммит в базу прошёл…
		t.Fatal(err)
	}
	// …смещение не зафиксировано, процесс перезапущен: повторная доставка.
	if err := e.uc.HandleOrders(ctx, completed); err != nil {
		t.Fatal(err)
	}
	refunded := orderEvent(t, uuid.NewString(), order, u, e.film, "refunded")
	if err := e.uc.HandleOrders(ctx, refunded); err != nil {
		t.Fatal(err)
	}
	// Опоздавшее старое событие (другой event_id, ревизия 1) не откатывает проекцию.
	late := orderEvent(t, uuid.NewString(), order, u, e.film, "charging")
	if err := e.uc.HandleOrders(ctx, late); err != nil {
		t.Fatal(err)
	}
	if f := e.feed(t, u); len(f) != 1 || f[0].Status != "refunded" {
		t.Fatalf("feed after crash/replay/late event: %+v", f)
	}
	var inbox int
	if err := e.db.QueryRow(ctx, `SELECT count(*) FROM inbox WHERE event_type=$1`, events.OrderStatusChanged).Scan(&inbox); err != nil {
		t.Fatal(err)
	}
	if inbox != 3 {
		t.Fatalf("inbox must hold 3 distinct events, got %d", inbox)
	}
}

// Битое событие — постоянная ошибка (в боевом консьюмере уйдёт в DLQ), и
// никаких следов в базе.
func TestOrderFeedRejectsBrokenEvent(t *testing.T) {
	e := setup(t)
	bad := events.Envelope{ID: uuid.NewString(), Type: events.OrderStatusChanged, Data: []byte(`{"user_id": 42`)}
	if err := e.uc.HandleOrders(context.Background(), bad); !errors.Is(err, bus.Permanent) {
		t.Fatalf("broken event must be a permanent error (→ DLQ), got %v", err)
	}
	// Корректный JSON, но user_id — не UUID: тоже постоянная ошибка, а не
	// бесконечный повтор.
	weird := orderEvent(t, uuid.NewString(), uuid.New(), uuid.New(), e.film, "completed")
	weird.Data = []byte(`{"order_id":"` + uuid.NewString() + `","user_id":"abc","kind":"purchase","status":"completed"}`)
	if err := e.uc.HandleOrders(context.Background(), weird); !errors.Is(err, bus.Permanent) {
		t.Fatalf("event with a non-UUID user_id must be permanent (→ DLQ), got %v", err)
	}
	for _, id := range []string{bad.ID, weird.ID} {
		var n int
		if err := e.owner.QueryRow(context.Background(), `SELECT count(*) FROM inbox WHERE event_id=$1`, id).Scan(&n); err != nil || n != 0 {
			t.Fatalf("broken event %s marked as handled: %d %v", id, n, err)
		}
	}
}

// Изоляция зрителей в проекции под непривилегированной ролью: чужие заказы
// не видны, а записать строку другому зрителю нельзя даже обработчику.
func TestOrderFeedIsolatedByRLS(t *testing.T) {
	e := setup(t)
	a, b := uuid.New(), uuid.New()
	if err := e.uc.HandleOrders(context.Background(), orderEvent(t, uuid.NewString(), uuid.New(), a, e.film, "completed")); err != nil {
		t.Fatal(err)
	}
	if f := e.feed(t, b); len(f) != 0 {
		t.Fatalf("viewer B sees A's orders: %+v", f)
	}
	var none int
	if err := e.db.QueryRow(context.Background(), `SELECT count(*) FROM order_feed`).Scan(&none); err != nil {
		t.Fatal(err)
	}
	if none != 0 {
		t.Fatalf("query without viewer context sees %d rows", none)
	}
	err := pgx.BeginFunc(context.Background(), e.db, func(tx pgx.Tx) error {
		ctx := context.Background()
		if _, err := tx.Exec(ctx, `SELECT set_config('plenka.user_id', $1, true)`, a.String()); err != nil {
			t.Fatal(err)
		}
		_, err := tx.Exec(ctx, `INSERT INTO order_feed(order_id, user_id, kind, status, revision, changed_at)
			VALUES ($1,$2,'purchase','completed',2,now())`, uuid.New(), b)
		return err
	})
	if c := pgkit.Code(err); c != "42501" {
		t.Fatalf("row for another viewer must be refused by WITH CHECK, code=%q err=%v", c, err)
	}
}

// Событие уже известного заказа, но с другим зрителем (ошибка издателя или
// подделка). Запись упирается в политику RLS чужой строки — это ошибка
// события, а не базы: в DLQ, а не бесконечный повтор, и проекция не тронута.
func TestForeignUserForExistingOrderGoesToDLQ(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	alice, bob, order := uuid.New(), uuid.New(), uuid.New()
	if err := e.uc.HandleOrders(ctx, orderEvent(t, uuid.NewString(), order, alice, e.film, "completed")); err != nil {
		t.Fatal(err)
	}
	err := e.uc.HandleOrders(ctx, orderEvent(t, uuid.NewString(), order, bob, e.film, "refunded"))
	if !bus.IsPermanent(err) {
		t.Fatalf("event that rewrites another viewer's order must be permanent (→ DLQ), got %v", err)
	}
	if f := e.feed(t, alice); len(f) != 1 || f[0].Status != "completed" {
		t.Fatalf("alice's order must stay intact: %+v", f)
	}
}
