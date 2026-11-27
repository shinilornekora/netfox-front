// Package isolation проверяет одиночное владение данными на уровне СУБД:
// роль одного сервиса не может читать и писать таблицы другого.
//
//	TEST_PG_ADMIN_DSN="postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable" go test ./isolation/
//
// Тест создаёт временную базу, применяет deploy/init.sql (роли и схемы, как
// в compose), создаёт таблицу в схеме billing от имени billing и пробует
// добраться до неё от имени ролей других сервисов — владельцев и ролей
// данных. Роль данных billing читает свои данные, но не может менять схему.
// Без переменной пропускается.
package isolation

import (
	"context"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestServiceRolesCannotTouchForeignSchemas(t *testing.T) {
	admin := os.Getenv("TEST_PG_ADMIN_DSN")
	if admin == "" {
		t.Skip("TEST_PG_ADMIN_DSN не задан")
	}
	ctx := context.Background()
	root, err := pgx.Connect(ctx, admin)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close(ctx)
	const db = "plenka_isolation_test"
	_, _ = root.Exec(ctx, `DROP DATABASE IF EXISTS `+db+` WITH (FORCE)`)
	if _, err := root.Exec(ctx, `CREATE DATABASE `+db); err != nil {
		t.Fatal(err)
	}
	defer root.Exec(context.Background(), `DROP DATABASE IF EXISTS `+db+` WITH (FORCE)`) //nolint:errcheck

	u, _ := url.Parse(admin)
	u.Path = "/" + db
	adm, err := pgx.Connect(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer adm.Close(ctx)
	sql, err := os.ReadFile("../../deploy/init.sql")
	if err != nil {
		t.Fatal(err)
	}
	// init.sql идемпотентен: роли кластерные и могут уже существовать.
	if _, err := adm.Exec(ctx, string(sql)); err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"identity", "catalog", "billing", "playback"} {
		_, _ = adm.Exec(ctx, `GRANT CONNECT ON DATABASE `+db+` TO `+s+`, `+s+`_app`)
	}

	as := func(role string) *pgx.Conn {
		v := *u
		v.User = url.UserPassword(role, role+"_pw")
		c, err := pgx.Connect(ctx, v.String())
		if err != nil {
			t.Fatalf("connect as %s: %v", role, err)
		}
		return c
	}
	billing := as("billing")
	defer billing.Close(ctx)
	if _, err := billing.Exec(ctx, `CREATE TABLE billing.orders_probe (id int PRIMARY KEY, amount int); INSERT INTO billing.orders_probe VALUES (1, 29900)`); err != nil {
		t.Fatal(err)
	}
	// Роль данных самого billing: данные — да (права выдаёт сервис при старте,
	// здесь — как pgkit.GrantApp), DDL в своей схеме — нет.
	if _, err := billing.Exec(ctx, `GRANT USAGE ON SCHEMA billing TO billing_app; GRANT SELECT, UPDATE ON billing.orders_probe TO billing_app`); err != nil {
		t.Fatal(err)
	}
	ba := as("billing_app")
	var amount int
	if err := ba.QueryRow(ctx, `SELECT amount FROM billing.orders_probe`).Scan(&amount); err != nil || amount != 29900 {
		t.Fatalf("billing_app must read its own data: %d %v", amount, err)
	}
	for _, q := range []string{`CREATE TABLE billing.intruder (x int)`, `ALTER TABLE billing.orders_probe ENABLE ROW LEVEL SECURITY`, `DROP TABLE billing.orders_probe`} {
		if _, err := ba.Exec(ctx, q); err == nil || !strings.Contains(err.Error(), "permission denied") && !strings.Contains(err.Error(), "must be owner") {
			t.Errorf("billing_app ran DDL %q: %v", q, err)
		}
	}
	ba.Close(ctx)

	for _, role := range []string{"catalog", "playback", "identity", "catalog_app", "playback_app", "identity_app"} {
		c := as(role)
		var n int
		err := c.QueryRow(ctx, `SELECT count(*) FROM billing.orders_probe`).Scan(&n)
		if err == nil || !strings.Contains(err.Error(), "permission denied") {
			t.Errorf("%s read billing: n=%d err=%v", role, n, err)
		}
		_, err = c.Exec(ctx, `UPDATE billing.orders_probe SET amount = 0`)
		if err == nil || !strings.Contains(err.Error(), "permission denied") {
			t.Errorf("%s wrote billing: %v", role, err)
		}
		_, err = c.Exec(ctx, `CREATE TABLE billing.intruder (x int)`)
		if err == nil || !strings.Contains(err.Error(), "permission denied") {
			t.Errorf("%s created a table in billing: %v", role, err)
		}
		c.Close(ctx)
	}
}
