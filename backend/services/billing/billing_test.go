package billing_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"plenka/billing/internal/adapter/paysim"
	"plenka/billing/internal/adapter/pgsql"
	"plenka/billing/internal/core"
	"plenka/billing/internal/usecase"
	"plenka/billing/migrations"
	"plenka/contracts/events"
	"plenka/kit/pgkit"
	"plenka/kit/pgtest"
	"plenka/kit/principal"
	"plenka/kit/problem"
)

const (
	okCard       = "4242 4242 4242 4242"
	declinedCard = "4000 0000 0000 0002"
	lostCard     = "4000 0000 0000 0259"
)

type env struct {
	db      *pgxpool.Pool // роль данных, как у сервиса
	owner   *pgxpool.Pool // владелец схемы: ретранслятор outbox
	store   pgsql.Store
	saga    *usecase.Saga
	account *usecase.Account
	film    uuid.UUID
	series  uuid.UUID
}

func setup(t *testing.T) *env {
	pools := pgtest.FreshPools(t, "billing_test", migrations.FS, migrations.Deny...)
	db := pools.App
	st := pgsql.Store{DB: db}
	e := &env{db: db, owner: pools.Owner, store: st, film: uuid.New(), series: uuid.New(),
		saga:    &usecase.Saga{Store: st, Provider: paysim.Gateway{DB: db}, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), ChargeTimeout: 300 * time.Millisecond},
		account: &usecase.Account{Ledger: st}}
	e.title(t, e.film, "film", "published", true, 29900, 1)
	e.title(t, e.series, "series", "published", true, 0, 1)
	return e
}

func (e *env) title(t *testing.T, id uuid.UUID, kind, status string, inSub bool, price int64, rev int64) {
	t.Helper()
	snap := events.TitleSnapshot{ID: id.String(), Kind: kind, Name: "Тайтл " + kind, Status: status, InSubscription: inSub, PriceKopecks: price, Revision: rev}
	data, _ := json.Marshal(snap)
	env := events.Envelope{ID: uuid.NewString(), Type: events.TitleUpdated, Revision: rev, Data: data}
	if err := e.account.HandleCatalog(context.Background(), env); err != nil {
		t.Fatal(err)
	}
}

func viewer(id uuid.UUID) context.Context {
	return principal.With(context.Background(), principal.Principal{UserID: id.String(), Role: principal.Viewer})
}

func (e *env) buy(user uuid.UUID, card, key string) (core.Order, bool, error) {
	return e.saga.Checkout(viewer(user), usecase.CheckoutInput{Kind: core.KindPurchase, TitleID: e.film, Card: card, Key: key})
}

func (e *env) subscribe(user uuid.UUID, card, key string) (core.Order, bool, error) {
	return e.saga.Checkout(viewer(user), usecase.CheckoutInput{Kind: core.KindSubscription, PlanID: "month", Card: card, Key: key})
}

func TestPurchaseHappyPathAndAccess(t *testing.T) {
	e := setup(t)
	u := uuid.New()
	ctx := context.Background()
	d, _, _, _, _, _ := e.account.Check(ctx, u, e.film)
	if d.Allowed {
		t.Fatal("no access before purchase")
	}
	o, _, err := e.buy(u, okCard, "key-purchase-1")
	if err != nil || o.Status != core.Completed || o.Amount != 29900 || o.CardLast4 != "4242" {
		t.Fatalf("purchase: %+v %v", o, err)
	}
	d, _, _, _, _, _ = e.account.Check(ctx, u, e.film)
	if !d.Allowed || d.Via != "purchase" {
		t.Fatalf("purchased film must be watchable: %+v", d)
	}
	if d, _, _, _, _, _ = e.account.Check(ctx, u, e.series); d.Allowed {
		t.Fatal("series needs a subscription")
	}
	if _, _, err := e.buy(u, okCard, "key-purchase-2"); !problem.Is(err, "already_owned") {
		t.Fatalf("second purchase of the same film: %v", err)
	}
	// Повтор того же запроса — тот же заказ, деньги второй раз не списаны.
	again, replay, err := e.buy(u, okCard, "key-purchase-1")
	if err != nil || !replay || again.ID != o.ID {
		t.Fatalf("idempotent replay: %+v %v %v", again, replay, err)
	}
	var charges int
	if err := e.db.QueryRow(ctx, `SELECT count(*) FROM paysim_charges`).Scan(&charges); err != nil {
		t.Fatal(err)
	}
	if charges != 1 {
		t.Fatalf("exactly one charge expected, got %d", charges)
	}
	// Снятый с показа тайтл недоступен даже купившему.
	e.title(t, e.film, "film", "withdrawn", true, 29900, 2)
	if d, _, _, _, _, _ = e.account.Check(ctx, u, e.film); d.Allowed || d.Denial != "not_available" {
		t.Fatalf("withdrawn title must be blocked: %+v", d)
	}
}

func TestSubscriptionGrantsSeriesAndCanBeCancelled(t *testing.T) {
	e := setup(t)
	u := uuid.New()
	o, _, err := e.subscribe(u, okCard, "key-sub-0001")
	if err != nil || o.Status != core.Completed {
		t.Fatalf("subscribe: %+v %v", o, err)
	}
	d, _, _, _, _, _ := e.account.Check(context.Background(), u, e.series)
	if !d.Allowed || d.Via != "subscription" || d.ValidUntil.Before(time.Now().AddDate(0, 0, 29)) {
		t.Fatalf("subscription must open series for ~30 days: %+v", d)
	}
	if _, _, err := e.subscribe(u, okCard, "key-sub-0002"); !problem.Is(err, "already_subscribed") {
		t.Fatalf("second subscription: %v", err)
	}
	s, err := e.account.SetAutoRenew(viewer(u), false)
	if err != nil || !s.CancelAtPeriodEnd {
		t.Fatalf("cancel auto-renew: %+v %v", s, err)
	}
	if d, _, _, _, _, _ = e.account.Check(context.Background(), u, e.series); !d.Allowed {
		t.Fatal("access stays until the end of the paid period")
	}
}

func TestDeclinedCard(t *testing.T) {
	e := setup(t)
	u := uuid.New()
	o, _, err := e.buy(u, declinedCard, "key-declined-1")
	if err != nil || o.Status != core.Failed || o.Failure == "" {
		t.Fatalf("declined: %+v %v", o, err)
	}
	if d, _, _, _, _, _ := e.account.Check(context.Background(), u, e.film); d.Allowed {
		t.Fatal("no access after decline")
	}
	// После отказа можно попробовать другой картой.
	if o, _, err := e.buy(u, okCard, "key-declined-2"); err != nil || o.Status != core.Completed {
		t.Fatalf("retry with another card: %+v %v", o, err)
	}
	if _, err := core.CleanCard("4242 4242 4242 4241"); !problem.Is(err, "invalid") {
		t.Fatal("Luhn check must reject a typo")
	}
}

// Write skew: десять параллельных оформлений подписки разными ключами.
// Каждая транзакция проверяет «нет заказа в работе и нет подписки» и
// вставляет заказ. Без SERIALIZABLE несколько прошли бы проверку
// одновременно и списали бы деньги несколько раз.
func TestConcurrentCheckoutWriteSkew(t *testing.T) {
	e := setup(t)
	u := uuid.New()
	var wg sync.WaitGroup
	var completed, rejected atomic.Int32
	start := make(chan struct{})
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			o, _, err := e.subscribe(u, okCard, uuid.NewString())
			switch {
			case err == nil && o.Status == core.Completed:
				completed.Add(1)
			case problem.Is(err, "order_in_progress"), problem.Is(err, "already_subscribed"):
				rejected.Add(1)
			default:
				t.Errorf("unexpected: %+v %v", o, err)
			}
		}(i)
	}
	close(start)
	wg.Wait()
	var charges, subs int
	ctx := context.Background()
	if err := e.db.QueryRow(ctx, `SELECT count(*) FROM paysim_charges`).Scan(&charges); err != nil {
		t.Fatal(err)
	}
	if err := e.db.QueryRow(ctx, `SELECT count(*) FROM subscriptions WHERE user_id=$1`, u).Scan(&subs); err != nil {
		t.Fatal(err)
	}
	if completed.Load() != 1 || charges != 1 || subs != 1 {
		t.Fatalf("want 1 completed / 1 charge / 1 subscription, got %d / %d / %d (rejected %d)",
			completed.Load(), charges, subs, rejected.Load())
	}
}

// Эксперимент РХД ПР-3 на покупке одного фильма: два одновременных
// оформления одной покупки (разные ключи — две вкладки). Обе транзакции
// читают «покупки нет, заказа в работе нет» и вставляют свой заказ.
// Точка синхронизации гарантирует, что обе прочитали до того, как кто-то
// вставил, — иначе гонка ловилась бы через раз.
//
//	READ COMMITTED → два заказа и два списания (аномалия write skew);
//	SERIALIZABLE   → одно списание, вторая транзакция откатывается,
//	                 повторяется и видит заказ в работе.
func TestPurchaseWriteSkewExperiment(t *testing.T) {
	for _, c := range []struct {
		name        string
		weak        bool
		wantCharges int
	}{
		{"READ COMMITTED", true, 2},
		{"SERIALIZABLE", false, 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := setup(t)
			var arrived atomic.Int32
			gate := make(chan struct{})
			var once sync.Once
			st := pgsql.Store{DB: e.db, WeakPrepare: c.weak, BeforeInsert: func() {
				if arrived.Add(1) == 2 {
					once.Do(func() { close(gate) })
				}
				select { // повтор после отката ждать не должен
				case <-gate:
				case <-time.After(2 * time.Second):
				}
			}}
			e.saga.Store = st
			u := uuid.New()
			var wg sync.WaitGroup
			for i := 0; i < 2; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					_, _, _ = e.buy(u, okCard, uuid.NewString())
				}()
			}
			wg.Wait()
			var charges, orders int
			ctx := context.Background()
			if err := e.db.QueryRow(ctx, `SELECT count(*) FROM paysim_charges WHERE status IN ('succeeded','refunded')`).Scan(&charges); err != nil {
				t.Fatal(err)
			}
			if err := e.db.QueryRow(ctx, `SELECT count(*) FROM orders WHERE user_id=$1`, u).Scan(&orders); err != nil {
				t.Fatal(err)
			}
			t.Logf("%s: заказов %d, списаний %d", c.name, orders, charges)
			if charges != c.wantCharges {
				t.Fatalf("want %d charges, got %d (orders %d)", c.wantCharges, charges, orders)
			}
		})
	}
}

// Регрессия, найденная нагрузочным замером: проверка «заказ в работе» в
// обобщённом плане шла по всему частичному индексу orders_stuck_idx, и
// покупки разных зрителей ложно конфликтовали в SERIALIZABLE. Проверяем план
// (именно обобщённый — так выполняются подготовленные запросы pgx) и то, что
// параллельные покупки разных зрителей проходят без ошибок.
func TestInFlightCheckUsesUserIndex(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	// План смотрим ровно в тех условиях, где выполняется проверка: внутри
	// pgkit.Serializable, на почти пустой таблице, обобщённым планом.
	var plan string
	err := pgkit.Serializable(ctx, e.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SET LOCAL plan_cache_mode = force_generic_plan`); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `PREPARE inflight(uuid, text) AS SELECT EXISTS(SELECT 1 FROM orders WHERE user_id=$1
			AND status IN ('charging','refunding')
			AND (CASE WHEN kind='subscription' THEN 'subscription' ELSE 'title:' || title_id::text END) = $2)`); err != nil {
			return err
		}
		defer tx.Exec(ctx, `DEALLOCATE inflight`) //nolint:errcheck
		rows, err := tx.Query(ctx, `EXPLAIN EXECUTE inflight('00000000-0000-0000-0000-000000000001', 'x')`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var line string
			if err := rows.Scan(&line); err != nil {
				return err
			}
			plan += line + "\n"
		}
		return rows.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan, "orders_user_status_idx") {
		t.Fatalf("in-flight check must use the per-user index:\n%s", plan)
	}

	// 64 покупки разных зрителей, 8 одновременно — как в замере. Клиент
	// ведёт себя как настоящий: на 503 contention повторяет запрос с тем же
	// Idempotency-Key. Если повтор исчерпался на шаге fulfil, заказ остаётся
	// в charging, и его доводит восстановитель — так же, как в сервисе.
	// Итог обязан быть «все 64 оплачены, ни одной ошибки»;
	// сколько раз понадобился повтор — видно в логе теста. На почти пустой
	// таблице (здесь) SSI блокирует страницами, и под -race, когда транзакции
	// длиннее, единичные повторы бывают; на 100 тыс. заказов в замере их ноль.
	var wg sync.WaitGroup
	var failed, retried atomic.Int32
	var mu sync.Mutex
	var pending []uuid.UUID
	jobs := make(chan struct{})
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range jobs {
				user, key := uuid.New(), uuid.NewString()
				var o core.Order
				var err error
				for attempt := 0; attempt < 5; attempt++ {
					if o, _, err = e.buy(user, okCard, key); !problem.Is(err, "contention") {
						break
					}
					retried.Add(1)
				}
				switch {
				case err == nil && o.Status == core.Charging:
					mu.Lock()
					pending = append(pending, o.ID)
					mu.Unlock()
				case err != nil || o.Status != core.Completed:
					failed.Add(1)
					t.Logf("purchase: %+v %v", o.Status, err)
				}
			}
		}()
	}
	for i := 0; i < 64; i++ {
		jobs <- struct{}{}
	}
	close(jobs)
	wg.Wait()
	if len(pending) > 0 {
		if _, err := e.saga.Recover(ctx, 0); err != nil {
			t.Fatal(err)
		}
		for _, id := range pending {
			if o, err := e.store.Order(ctx, id); err != nil || o.Status != core.Completed {
				failed.Add(1)
				t.Logf("recovered order: %+v %v", o.Status, err)
			}
		}
	}
	t.Logf("retries after 503 contention: %d, finished by recovery: %d", retried.Load(), len(pending))
	if failed.Load() != 0 {
		t.Fatalf("%d of 64 parallel purchases by different viewers failed", failed.Load())
	}
}

// Ответ банка потерян: деньги списаны, но клиент получил тайм-аут. Заказ
// остаётся charging; recovery сверяется со шлюзом и доводит сагу.
func TestLostResponseIsRecovered(t *testing.T) {
	e := setup(t)
	u := uuid.New()
	o, _, err := e.buy(u, lostCard, "key-lost-001")
	if err != nil || o.Status != core.Charging {
		t.Fatalf("unknown outcome must leave order charging: %+v %v", o, err)
	}
	if d, _, _, _, _, _ := e.account.Check(context.Background(), u, e.film); d.Allowed {
		t.Fatal("no access while payment outcome is unknown")
	}
	n, err := e.saga.Recover(context.Background(), -time.Second)
	if err != nil || n != 1 {
		t.Fatalf("recover: n=%d err=%v", n, err)
	}
	got, _ := e.store.Order(context.Background(), o.ID)
	if got.Status != core.Completed {
		t.Fatalf("recovered order must be completed, got %s", got.Status)
	}
	if d, _, _, _, _, _ := e.account.Check(context.Background(), u, e.film); !d.Allowed {
		t.Fatal("access after recovery")
	}
}

// interfering — шлюз, во время списания которого у зрителя «параллельно»
// появляется подписка (другой заказ, оформленный с другого устройства).
type interfering struct {
	paysim.Gateway
	db   *pgxpool.Pool
	user uuid.UUID
}

func (g interfering) Charge(ctx context.Context, ref string, amount int64, card core.Card) (usecase.ChargeResult, error) {
	res, err := g.Gateway.Charge(ctx, ref, amount, card)
	orderID := uuid.New()
	_, _ = g.db.Exec(ctx, `INSERT INTO orders(id, user_id, kind, plan_id, amount_kopecks, status, card_last4, idem_key, request_hash)
		VALUES ($1,$2,'subscription','year',100,'completed','0000',$3,'x')`, orderID, g.user, uuid.NewString())
	_, _ = g.db.Exec(ctx, `INSERT INTO subscriptions(id, user_id, plan_id, order_id, status, started_at, current_period_end)
		VALUES ($1,$2,'year',$3,'active',now(),now() + interval '365 days')`, uuid.New(), g.user, orderID)
	return res, err
}

// Компенсация: деньги списали, а выдать подписку нельзя (она появилась
// другим путём). Оркестратор возвращает деньги: refunding → refunded.
func TestCompensationRefunds(t *testing.T) {
	e := setup(t)
	u := uuid.New()
	e.saga.Provider = interfering{Gateway: paysim.Gateway{DB: e.db}, db: e.db, user: u}
	o, _, err := e.subscribe(u, okCard, "key-comp-001")
	if err != nil || o.Status != core.Refunded {
		t.Fatalf("expected refunded order, got %+v %v", o, err)
	}
	var st string
	if err := e.db.QueryRow(context.Background(), `SELECT status FROM paysim_charges WHERE ref=$1`, o.ID.String()).Scan(&st); err != nil {
		t.Fatal(err)
	}
	if st != "refunded" {
		t.Fatalf("money must be returned at the gateway, status %q", st)
	}
}

func TestReplicaIgnoresStaleRevision(t *testing.T) {
	e := setup(t)
	e.title(t, e.film, "film", "withdrawn", true, 29900, 5)
	e.title(t, e.film, "film", "published", true, 29900, 3) // устаревшее событие
	if _, _, err := e.buy(uuid.New(), okCard, "key-stale-01"); !problem.Is(err, "not_for_sale") {
		t.Fatalf("stale event must not republish the film: %v", err)
	}
}

func admin() context.Context {
	return principal.With(context.Background(), principal.Principal{UserID: uuid.NewString(), Role: principal.Admin})
}

// Возврат оплаченной покупки: право забирается в той же транзакции, что
// completed → refunding, деньги возвращаются, в outbox — два события.
func TestAdminRefundRevokesPurchase(t *testing.T) {
	e := setup(t)
	u := uuid.New()
	ctx := context.Background()
	o, _, err := e.buy(u, okCard, "key-refund-1")
	if err != nil || o.Status != core.Completed {
		t.Fatalf("buy: %+v %v", o, err)
	}
	if _, err := e.saga.Refund(viewer(u), o.ID, ""); !problem.Is(err, "forbidden") {
		t.Fatalf("viewer must not refund: %v", err)
	}
	r, err := e.saga.Refund(admin(), o.ID, "Жалоба на качество")
	if err != nil || r.Status != core.Refunded || r.Failure != "Жалоба на качество" {
		t.Fatalf("refund: %+v %v", r, err)
	}
	if d, _, _, _, _, _ := e.account.Check(ctx, u, e.film); d.Allowed {
		t.Fatalf("refunded film must not play: %+v", d)
	}
	var charge string
	if err := e.db.QueryRow(ctx, `SELECT status FROM paysim_charges WHERE ref=$1`, o.ID.String()).Scan(&charge); err != nil {
		t.Fatal(err)
	}
	if charge != "refunded" {
		t.Fatalf("bank must refund, charge status %q", charge)
	}
	var got []string
	for _, ev := range e.events(t, o.ID) {
		got = append(got, ev.Status)
	}
	if strings.Join(got, ",") != "charging,completed,refunding,refunded" {
		t.Fatalf("outbox events: %v", got)
	}
	// Повтор возврата ничего не делает; возврат неоплаченного — 409.
	if again, err := e.saga.Refund(admin(), o.ID, ""); err != nil || again.Status != core.Refunded {
		t.Fatalf("repeat refund: %+v %v", again, err)
	}
	failed, _, _ := e.buy(uuid.New(), declinedCard, "key-refund-2")
	if _, err := e.saga.Refund(admin(), failed.ID, ""); !problem.Is(err, "bad_state") {
		t.Fatalf("refund of a failed order: %v", err)
	}
	// Купить снова можно — запрет двойной покупки смотрит на действующее право.
	if again, _, err := e.buy(u, okCard, "key-refund-3"); err != nil || again.Status != core.Completed {
		t.Fatalf("buy after refund: %+v %v", again, err)
	}
}

func TestAdminRefundEndsSubscription(t *testing.T) {
	e := setup(t)
	u := uuid.New()
	o, _, err := e.subscribe(u, okCard, "key-sub-refund")
	if err != nil || o.Status != core.Completed {
		t.Fatalf("subscribe: %+v %v", o, err)
	}
	if d, _, _, _, _, _ := e.account.Check(context.Background(), u, e.series); !d.Allowed {
		t.Fatal("subscription must open the series")
	}
	if r, err := e.saga.Refund(admin(), o.ID, ""); err != nil || r.Status != core.Refunded {
		t.Fatalf("refund: %+v %v", r, err)
	}
	if d, _, _, _, _, _ := e.account.Check(context.Background(), u, e.series); d.Allowed {
		t.Fatalf("refunded subscription must close the series: %+v", d)
	}
}
