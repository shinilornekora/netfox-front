package pgkit

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Pools — две роли сервиса (TRADEOFFS 3.8).
//
//   - Owner владеет схемой: миграции, обслуживание секций, ретранслятор
//     outbox. Только владелец может отключить RLS (ALTER TABLE … NO FORCE).
//   - App — роль запросов пользователей: только SELECT/INSERT/UPDATE/DELETE
//     на таблицах схемы, без DDL. Снять RLS под ней нельзя — ни ошибкой,
//     ни SQL-инъекцией. Выставить чужой контекст зрителя — можно: от этого
//     защищает подписанная личность на уровне приложения (TRADEOFFS 3.8).
//
// Если APP_DATABASE_URL не задан, обе роли — одна (старый режим), и сервис
// предупреждает об этом в логе.
type Pools struct {
	Owner, App *pgxpool.Pool
}

func (p Pools) Close() {
	if p.App != p.Owner {
		p.App.Close()
	}
	p.Owner.Close()
}

// Deny — права, которые роль App не получает на конкретной таблице,
// например {Table: "outbox", Privs: "SELECT, UPDATE, DELETE"}: сервис только
// добавляет события, читает и отмечает их ретранслятор под ролью владельца.
type Deny struct{ Table, Privs string }

// OpenPools подключает владельца, применяет миграции, затем подключает App
// и выдаёт ей права на текущую схему. Права выдаются при каждом старте —
// это идемпотентно и покрывает таблицы из новых миграций.
func OpenPools(ctx context.Context, log *slog.Logger, ownerDSN, appDSN string, appConns int32, migrations fs.FS, deny ...Deny) (Pools, error) {
	ownerConns := int32(4)
	if appDSN == "" {
		ownerConns = appConns
	}
	owner, err := Open(ctx, log, ownerDSN, ownerConns)
	if err != nil {
		return Pools{}, err
	}
	if err := Migrate(ctx, owner, migrations); err != nil {
		owner.Close()
		return Pools{}, err
	}
	if appDSN == "" {
		log.Warn("APP_DATABASE_URL not set: requests run under the schema owner role")
		return Pools{Owner: owner, App: owner}, nil
	}
	app, err := Open(ctx, log, appDSN, appConns)
	if err != nil {
		owner.Close()
		return Pools{}, err
	}
	var role string
	if err := app.QueryRow(ctx, `SELECT current_user`).Scan(&role); err != nil {
		app.Close()
		owner.Close()
		return Pools{}, err
	}
	if err := GrantApp(ctx, owner, role, deny...); err != nil {
		app.Close()
		owner.Close()
		return Pools{}, fmt.Errorf("grant %s: %w", role, err)
	}
	return Pools{Owner: owner, App: app}, nil
}

// GrantApp выдаёт роли app права на данные текущей схемы владельца.
func GrantApp(ctx context.Context, owner *pgxpool.Pool, app string, deny ...Deny) error {
	r := pgx.Identifier{app}.Sanitize()
	return pgx.BeginFunc(ctx, owner, func(tx pgx.Tx) error {
		var schema string
		if err := tx.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema); err != nil {
			return err
		}
		s := pgx.Identifier{schema}.Sanitize()
		stmts := []string{
			`GRANT USAGE ON SCHEMA ` + s + ` TO ` + r,
			`GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA ` + s + ` TO ` + r,
			`GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA ` + s + ` TO ` + r,
			`ALTER DEFAULT PRIVILEGES IN SCHEMA ` + s + ` GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO ` + r,
			`ALTER DEFAULT PRIVILEGES IN SCHEMA ` + s + ` GRANT USAGE, SELECT ON SEQUENCES TO ` + r,
			`REVOKE ALL ON ` + s + `.kit_migrations FROM ` + r,
		}
		for _, d := range deny {
			stmts = append(stmts, `REVOKE `+d.Privs+` ON `+s+`.`+pgx.Identifier{d.Table}.Sanitize()+` FROM `+r)
		}
		// Секции секционированных таблиц не наследуют RLS родителя: роль
		// данных обращается к ним только через родителя, где действует политика.
		rows, err := tx.Query(ctx, `SELECT c.relname FROM pg_class c
			WHERE c.relnamespace = $1::regnamespace AND c.relispartition AND c.relkind IN ('r', 'p')`, schema)
		if err != nil {
			return err
		}
		parts, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return err
		}
		for _, p := range parts {
			stmts = append(stmts, `REVOKE ALL ON `+s+`.`+pgx.Identifier{p}.Sanitize()+` FROM `+r)
		}
		for _, q := range stmts {
			if _, err := tx.Exec(ctx, q); err != nil {
				return fmt.Errorf("%s: %w", q, err)
			}
		}
		return nil
	})
}
