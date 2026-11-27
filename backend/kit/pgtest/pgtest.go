// Package pgtest — интеграционные тесты на настоящем PostgreSQL.
//
//	TEST_PG_DSN="postgres://plenka_test:plenka_test@localhost:5432/plenka_test?sslmode=disable"
//	TEST_PG_APP_DSN="postgres://plenka_test_app:plenka_test_app@localhost:5432/plenka_test?sslmode=disable"
//
// TEST_PG_DSN — роль-владелец схемы (миграции). Если задан TEST_PG_APP_DSN,
// тесты работают под отдельной ролью данных, как сервис в бою (pgkit.Pools),
// и проверяют, что она не может снять RLS.
//
// Каждый тест получает чистую схему (как сервис в бою — свою схему).
// Роль не должна быть суперпользователем, иначе RLS не проверяется.
package pgtest

import (
	"context"
	"io"
	"io/fs"
	"log/slog"
	"net/url"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"plenka/kit/pgkit"
)

// Fresh — пул, под которым работает код сервиса (роль данных, если она
// задана, иначе владелец).
func Fresh(t *testing.T, schema string, migrations fs.FS, deny ...pgkit.Deny) *pgxpool.Pool {
	t.Helper()
	return FreshPools(t, schema, migrations, deny...).App
}

// Split — заданы ли разные роли владельца и данных.
func Split() bool { return os.Getenv("TEST_PG_APP_DSN") != "" }

func withSchema(dsn, schema string) string {
	u, _ := url.Parse(dsn)
	q := u.Query()
	q.Set("search_path", schema+",public")
	u.RawQuery = q.Encode()
	return u.String()
}

func FreshPools(t *testing.T, schema string, migrations fs.FS, deny ...pgkit.Deny) pgkit.Pools {
	t.Helper()
	base := os.Getenv("TEST_PG_DSN")
	if base == "" {
		t.Skip("TEST_PG_DSN not set")
	}
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	admin, err := pgkit.Open(ctx, log, base, 2)
	if err != nil {
		t.Fatal(err)
	}
	var super bool
	_ = admin.QueryRow(ctx, `SELECT rolsuper FROM pg_roles WHERE rolname = current_user`).Scan(&super)
	if super {
		t.Fatal("use a non-superuser role: superusers bypass RLS")
	}
	if _, err := admin.Exec(ctx, `DROP SCHEMA IF EXISTS `+schema+` CASCADE; CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	admin.Close()
	appDSN := os.Getenv("TEST_PG_APP_DSN")
	if appDSN != "" {
		appDSN = withSchema(appDSN, schema)
	}
	pools, err := pgkit.OpenPools(ctx, log, withSchema(base, schema), appDSN, 30, migrations, deny...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pools.Close)
	return pools
}
