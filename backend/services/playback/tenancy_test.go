package playback_test

// РХД ПР-2, требование к домашней системе: два независимых уровня
// принуждения и не меньше трёх отрицательных тестов.
//   Уровень 1 — приложение: зритель берётся только из подписанной личности
//               (principal), идентификатор пользователя в API не передаётся.
//   Уровень 2 — PostgreSQL RLS с FORCE под ролью данных без BYPASSRLS,
//               которая не владеет таблицами и не может снять политику
//               (отрицательный 5, нужен TEST_PG_APP_DSN).
// Модель размещения — общая (одна схема, строки разных зрителей в одной
// таблице), обоснование — docs/TRADEOFFS.md 2.19.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"plenka/kit/pgkit"
	"plenka/kit/pgtest"
	"plenka/kit/problem"
)

func asViewer(t *testing.T, e *env, user uuid.UUID, sql string, args ...any) (int64, error) {
	t.Helper()
	ctx := context.Background()
	tx, err := e.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx, `SELECT set_config('plenka.user_id', $1, true)`, user.String()); err != nil {
		t.Fatal(err)
	}
	tag, err := tx.Exec(ctx, sql, args...)
	return tag.RowsAffected(), err
}

func (e *env) aliceWatches(t *testing.T) uuid.UUID {
	t.Helper()
	alice := uuid.New()
	if _, err := e.uc.Play(viewer(alice, 1990), e.film, nil); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.uc.SaveProgress(viewer(alice, 1990), e.film, nil, 600, 6000, time.Now().UnixMilli()); err != nil {
		t.Fatal(err)
	}
	if err := e.uc.AddWatch(viewer(alice, 1990), e.show); err != nil {
		t.Fatal(err)
	}
	if err := e.uc.HandleOrders(context.Background(), orderEvent(t, uuid.NewString(), uuid.New(), alice, e.film, "completed")); err != nil {
		t.Fatal(err)
	}
	return alice
}

var viewerTables = []string{"progress", "views", "watchlist", "order_feed"}

// В каждой таблице у Алисы есть строка — иначе «Боб видит ноль» ничего
// не доказывает.
func (e *env) aliceSeesOwn(t *testing.T, alice uuid.UUID) {
	t.Helper()
	for _, tbl := range viewerTables {
		if n, err := asViewer(t, e, alice, `SELECT * FROM `+tbl); err != nil || n < 1 {
			t.Fatalf("alice must see her own %s, got %d rows, %v", tbl, n, err)
		}
	}
}

// Отрицательный 1: другой зритель не читает чужой прогресс и историю.
func TestNegativeOtherViewerReadsNothing(t *testing.T) {
	e := setup(t)
	alice := e.aliceWatches(t)
	e.aliceSeesOwn(t, alice)
	bob := uuid.New()
	for _, tbl := range viewerTables {
		if n, err := asViewer(t, e, bob, `SELECT * FROM `+tbl); err != nil || n != 0 {
			t.Errorf("%s as bob: %d rows, %v", tbl, n, err)
		}
	}
}

// Отрицательный 2: чужие строки нельзя изменить или удалить, а новую строку
// нельзя записать от имени другого зрителя.
func TestNegativeCannotWriteForAnother(t *testing.T) {
	e := setup(t)
	alice := e.aliceWatches(t)
	bob := uuid.New()
	if n, err := asViewer(t, e, bob, `UPDATE progress SET position_sec = 0`); err != nil || n != 0 {
		t.Errorf("bob updated alice's progress: %d %v", n, err)
	}
	if n, err := asViewer(t, e, bob, `DELETE FROM views`); err != nil || n != 0 {
		t.Errorf("bob deleted alice's history: %d %v", n, err)
	}
	if n, err := asViewer(t, e, bob, `DELETE FROM watchlist`); err != nil || n != 0 {
		t.Errorf("bob deleted alice's watchlist: %d %v", n, err)
	}
	if n, err := asViewer(t, e, bob, `UPDATE order_feed SET status = 'refunded'`); err != nil || n != 0 {
		t.Errorf("bob changed alice's orders: %d %v", n, err)
	}
	for _, q := range []string{
		`INSERT INTO watchlist(user_id, title_id) VALUES ($1, $2)`,
		`INSERT INTO progress(user_id, title_id, position_sec, duration_sec, client_ms) VALUES ($1, $2, 1, 100, 1)`,
		`INSERT INTO order_feed(order_id, user_id, kind, status, revision, changed_at) VALUES (gen_random_uuid(), $1, 'purchase', 'completed', 2, now())`,
	} {
		args := []any{alice, e.film}
		if strings.Contains(q, "order_feed") {
			args = args[:1]
		}
		if _, err := asViewer(t, e, bob, q, args...); pgkit.Code(err) != "42501" {
			t.Errorf("bob wrote a row for alice (want 42501 from WITH CHECK): %s: %v", q, err)
		}
	}
}

// Отрицательный 3: забытый контекст — ноль строк, а не все строки (fail closed).
func TestNegativeNoContextSeesNothing(t *testing.T) {
	e := setup(t)
	e.aliceSeesOwn(t, e.aliceWatches(t))
	ctx := context.Background()
	for _, tbl := range viewerTables {
		var n int
		if err := e.db.QueryRow(ctx, `SELECT count(*) FROM `+tbl).Scan(&n); err != nil || n != 0 {
			t.Errorf("%s without context: %d rows, %v", tbl, n, err)
		}
	}
}

// Отрицательный 4 (уровень приложения): без подписанной личности сценарии
// зрителя не выполняются вовсе — до базы запрос не доходит.
func TestNegativeAnonymousRejectedBeforeDB(t *testing.T) {
	e := setup(t)
	if _, _, err := e.uc.Watchlist(context.Background()); !problem.Is(err, "unauthenticated") {
		t.Fatalf("anonymous watchlist: %v", err)
	}
	if _, err := e.uc.Play(context.Background(), e.film, nil); !problem.Is(err, "unauthenticated") {
		t.Fatalf("anonymous play: %v", err)
	}
}

// Отрицательный 5: роль данных не может снять защиту — ни отключить RLS,
// ни убрать FORCE, ни удалить политику, ни сменить владельца. Это делает
// только владелец схемы, под которым работают одни миграции.
func TestNegativeDataRoleCannotDisableRLS(t *testing.T) {
	if !pgtest.Split() {
		t.Skip("TEST_PG_APP_DSN не задан: роль данных совпадает с владельцем")
	}
	e := setup(t)
	ctx := context.Background()
	for _, q := range []string{
		`ALTER TABLE progress DISABLE ROW LEVEL SECURITY`,
		`ALTER TABLE progress NO FORCE ROW LEVEL SECURITY`,
		`DROP POLICY own_progress ON progress`,
		`ALTER TABLE order_feed OWNER TO CURRENT_USER`,
		`CREATE POLICY open_all ON watchlist USING (true)`,
	} {
		_, err := e.db.Exec(ctx, q)
		if c := pgkit.Code(err); c != "42501" {
			t.Errorf("data role ran %q: code=%q err=%v", q, c, err)
		}
	}
	var bypass bool
	if err := e.db.QueryRow(ctx, `SELECT rolbypassrls OR rolsuper FROM pg_roles WHERE rolname = current_user`).Scan(&bypass); err != nil {
		t.Fatal(err)
	}
	if bypass {
		t.Fatal("data role must not bypass RLS")
	}
}

// Отрицательный 6: секции истории не обходят RLS. В PostgreSQL секция не
// наследует политику родителя, поэтому у каждой секции своя такая же
// политика с FORCE (миграция 007), а роль данных к секциям напрямую не
// допускается вовсе.
func TestNegativePartitionsDoNotBypassRLS(t *testing.T) {
	e := setup(t)
	alice := e.aliceWatches(t)
	ctx := context.Background()
	part := "views_" + time.Now().UTC().Format("2006_01")
	// Слой 1: роль данных к секции напрямую не допущена.
	if pgtest.Split() {
		var n int
		err := e.db.QueryRow(ctx, `SELECT count(*) FROM `+part).Scan(&n)
		if c := pgkit.Code(err); c != "42501" {
			t.Fatalf("data role read partition %s directly: n=%d code=%q err=%v", part, n, c, err)
		}
	}
	// Слой 2: даже владелец без контекста зрителя видит в секции ноль строк,
	// а с контекстом Алисы — только её строки.
	var n int
	if err := e.owner.QueryRow(ctx, `SELECT count(*) FROM `+part).Scan(&n); err != nil || n != 0 {
		t.Fatalf("partition %s without context: %d rows, %v", part, n, err)
	}
	err := pgx.BeginFunc(ctx, e.owner, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT set_config('plenka.user_id', $1, true)`, alice.String()); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT count(*) FROM `+part).Scan(&n)
	})
	if err != nil || n != 1 {
		t.Fatalf("alice must see exactly her row in %s: %d, %v", part, n, err)
	}
	// Новые секции, которые создаёт обслуживание, защищены так же.
	if _, err := e.owner.Exec(ctx, `SELECT ensure_view_partitions(6)`); err != nil {
		t.Fatal(err)
	}
	// …в том числе без прав для роли данных сразу, а не со следующего старта.
	if pgtest.Split() {
		fresh := "views_" + time.Now().UTC().AddDate(0, 6, 0).Format("2006_01")
		var role string
		var can bool
		if err := e.db.QueryRow(ctx, `SELECT current_user`).Scan(&role); err != nil {
			t.Fatal(err)
		}
		if err := e.owner.QueryRow(ctx, `SELECT has_table_privilege($1, $2, 'SELECT')`, role, fresh).Scan(&can); err != nil || can {
			t.Fatalf("data role got SELECT on freshly created %s: %v %v", fresh, can, err)
		}
	}
	var unprotected int
	if err := e.owner.QueryRow(ctx, `SELECT count(*) FROM pg_inherits i
		JOIN pg_class c ON c.oid = i.inhrelid JOIN pg_class p ON p.oid = i.inhparent
		WHERE p.relname = 'views' AND p.relnamespace = current_schema()::regnamespace
		  AND NOT (c.relrowsecurity AND c.relforcerowsecurity)`).Scan(&unprotected); err != nil || unprotected != 0 {
		t.Fatalf("%d partitions without forced RLS, %v", unprotected, err)
	}
}
