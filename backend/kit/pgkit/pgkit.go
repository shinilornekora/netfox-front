// Package pgkit — PostgreSQL: подключение к схеме сервиса, миграции с
// контрольными суммами, транзакции SERIALIZABLE с повтором.
package pgkit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"math/rand/v2"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"plenka/kit/problem"
)

// Open подключается и ждёт готовности БД (до 60 с). Схема сервиса задаётся
// в DSN (search_path) — сервис физически работает в своей схеме.
func Open(ctx context.Context, log *slog.Logger, dsn string, maxConns int32) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	if maxConns > 0 {
		cfg.MaxConns = maxConns
	}
	cfg.ConnConfig.RuntimeParams["statement_timeout"] = "4000"
	cfg.ConnConfig.RuntimeParams["lock_timeout"] = "2000"
	deadline := time.Now().Add(time.Minute)
	for i := 1; ; i++ {
		pool, err := pgxpool.NewWithConfig(ctx, cfg)
		if err == nil {
			if err = pool.Ping(ctx); err == nil {
				return pool, nil
			}
			pool.Close()
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("postgres: %w", err)
		}
		log.Warn("waiting for postgres", "attempt", i, "err", err)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Second * time.Duration(min(i, 4))):
		}
	}
}

// Migrate применяет файлы NNN_*.sql по порядку и запоминает их контрольную
// сумму. Если уже применённый файл изменили — старт падает: правка старой
// миграции на работающей базе молча разошлась бы со схемой.
func Migrate(ctx context.Context, pool *pgxpool.Pool, fsys fs.FS) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock(hashtext(current_schema()))`); err != nil {
		return err
	}
	defer conn.Exec(context.Background(), `SELECT pg_advisory_unlock(hashtext(current_schema()))`) //nolint:errcheck
	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS kit_migrations(
		name text PRIMARY KEY, checksum text NOT NULL, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	files, err := fs.Glob(fsys, "*.sql")
	if err != nil {
		return err
	}
	sort.Strings(files)
	for _, f := range files {
		body, err := fs.ReadFile(fsys, path.Clean(f))
		if err != nil {
			return err
		}
		sum := sha256.Sum256(body)
		check := hex.EncodeToString(sum[:])
		var stored string
		err = conn.QueryRow(ctx, `SELECT checksum FROM kit_migrations WHERE name=$1`, f).Scan(&stored)
		switch {
		case err == nil && stored != check:
			return fmt.Errorf("migration %s was modified after being applied", f)
		case err == nil:
			continue
		case !errors.Is(err, pgx.ErrNoRows):
			return err
		}
		err = pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, string(body)); err != nil {
				return fmt.Errorf("%s: %w", f, err)
			}
			_, err := tx.Exec(ctx, `INSERT INTO kit_migrations(name, checksum) VALUES ($1,$2)`, f, check)
			return err
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// Retryable — serialization_failure или deadlock_detected.
func Retryable(err error) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && (pe.Code == "40001" || pe.Code == "40P01")
}

// Code возвращает SQLSTATE ошибки (или "").
func Code(err error) string {
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		return pe.Code
	}
	return ""
}

// Constraint — имя нарушенного ограничения.
func Constraint(err error) string {
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		return pe.ConstraintName
	}
	return ""
}

// Serializable выполняет fn в транзакции SERIALIZABLE и повторяет её при
// ошибке сериализации (до 6 попыток, экспоненциальная пауза со случайной
// добавкой). PostgreSQL сам находит write skew и откатывает одну из
// транзакций — код не обязан знать, какие строки блокировать.
//
// Внутри транзакции запрещено последовательное сканирование (SET LOCAL
// enable_seqscan = off), как советует документация PostgreSQL для SSI
// («Serializable Isolation Level»): последовательное сканирование ставит
// предикатную блокировку на всю таблицу, и тогда конфликтуют транзакции,
// которые трогают разные строки. Особенно заметно на пустой или маленькой
// таблице, где планировщик почти всегда выбирает Seq Scan. Найдено
// нагрузочным замером, см. docs/evidence/01-workload.md.
func Serializable(ctx context.Context, pool *pgxpool.Pool, fn func(pgx.Tx) error) error {
	return run(ctx, pool, pgx.TxOptions{IsoLevel: pgx.Serializable}, 6, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SET LOCAL enable_seqscan = off`); err != nil {
			return err
		}
		return fn(tx)
	})
}

// ReadCommitted — обычная транзакция; повтор только при deadlock.
func ReadCommitted(ctx context.Context, pool *pgxpool.Pool, fn func(pgx.Tx) error) error {
	return run(ctx, pool, pgx.TxOptions{IsoLevel: pgx.ReadCommitted}, 3, fn)
}

func run(ctx context.Context, pool *pgxpool.Pool, opts pgx.TxOptions, attempts int, fn func(pgx.Tx) error) error {
	var err error
	for i := 0; i < attempts; i++ {
		err = pgx.BeginTxFunc(ctx, pool, opts, fn)
		if !Retryable(err) {
			return err
		}
		back := time.Duration(1<<i)*5*time.Millisecond + time.Duration(rand.IntN(10))*time.Millisecond
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(back):
		}
	}
	// Все попытки исчерпаны конфликтами — это перегрузка, а не ошибка
	// сервера: 503, клиент повторяет с тем же Idempotency-Key.
	return problem.Unavailable("contention", "Сервис занят, повторите через секунду").With(err)
}

// AsUser выставляет контекст RLS до конца транзакции (SET LOCAL).
func AsUser(ctx context.Context, tx pgx.Tx, userID string) error {
	_, err := tx.Exec(ctx, `SELECT set_config('plenka.user_id', $1, true)`, userID)
	return err
}

// Q — общий интерфейс пула и транзакции.
type Q interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Like экранирует %, _ и \ для ILIKE.
func Like(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}
