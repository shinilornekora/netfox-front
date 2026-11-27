// catalog — тайтлы, серии, публикация.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"plenka/catalog/internal/adapter/pgsql"
	"plenka/catalog/internal/adapter/rest"
	"plenka/catalog/internal/usecase"
	"plenka/catalog/migrations"
	"plenka/contracts/events"
	"plenka/kit/bus"
	"plenka/kit/env"
	"plenka/kit/outbox"
	"plenka/kit/pgkit"
	"plenka/kit/principal"
	"plenka/kit/web"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "catalog:", err)
		os.Exit(1)
	}
}

func run() error {
	var e env.Reader
	addr := e.Str("HTTP_ADDR", ":8080")
	dsn := e.Must("DATABASE_URL")
	appDSN := e.Str("APP_DATABASE_URL", "")
	key := e.Must("INTERNAL_KEY")
	brokers := e.List("KAFKA_BROKERS", "localhost:9092")
	if err := e.Err(); err != nil {
		return err
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("svc", "catalog")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Две роли: владелец схемы (миграции, обслуживание, outbox) и роль
	// данных для запросов — она не может снять RLS (TRADEOFFS 3.8).
	pools, err := pgkit.OpenPools(ctx, log, dsn, appDSN, 10, migrations.FS, migrations.Deny...)
	if err != nil {
		return err
	}
	defer pools.Close()
	db := pools.App

	bus.EnsureTopics(ctx, brokers, log, 3, events.TopicTitles)
	prod, err := bus.NewProducer(brokers)
	if err != nil {
		return err
	}
	defer prod.Close()
	relay := &outbox.Relay{Pool: pools.Owner, Pub: prod, Log: log}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); relay.Run(ctx) }()

	uc := &usecase.Service{Store: pgsql.Store{DB: db}}
	health := &web.Health{Check: db.Ping}
	r := web.Secured(log, principal.Signer{Key: []byte(key)})
	health.Mount(r)
	(&rest.API{UC: uc}).Mount(r, web.Adapt(log))
	err = web.Serve(ctx, log, addr, r, health, 2*time.Second, 10*time.Second)
	stop()
	wg.Wait()
	return err
}
