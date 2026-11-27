// billing — подписки, покупки, сага оплаты, решение о доступе.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"google.golang.org/grpc"

	"plenka/billing/internal/adapter/paysim"
	"plenka/billing/internal/adapter/pgsql"
	"plenka/billing/internal/adapter/rest"
	"plenka/billing/internal/adapter/rpc"
	"plenka/billing/internal/usecase"
	"plenka/billing/migrations"
	"plenka/contracts/events"
	billingv1 "plenka/contracts/gen/billing/v1"
	"plenka/kit/bus"
	"plenka/kit/env"
	"plenka/kit/outbox"
	"plenka/kit/pgkit"
	"plenka/kit/principal"
	"plenka/kit/web"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "billing:", err)
		os.Exit(1)
	}
}

func run() error {
	var e env.Reader
	addr := e.Str("HTTP_ADDR", ":8080")
	grpcAddr := e.Str("GRPC_ADDR", ":9090")
	dsn := e.Must("DATABASE_URL")
	appDSN := e.Str("APP_DATABASE_URL", "")
	key := e.Must("INTERNAL_KEY")
	brokers := e.List("KAFKA_BROKERS", "localhost:9092")
	chargeTimeout := e.Dur("CHARGE_TIMEOUT", 3*time.Second)
	if err := e.Err(); err != nil {
		return err
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("svc", "billing")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Две роли: владелец схемы (миграции, обслуживание, outbox) и роль
	// данных для запросов — она не может снять RLS (TRADEOFFS 3.8).
	pools, err := pgkit.OpenPools(ctx, log, dsn, appDSN, 15, migrations.FS, migrations.Deny...)
	if err != nil {
		return err
	}
	defer pools.Close()
	db := pools.App

	store := pgsql.Store{DB: db}
	saga := &usecase.Saga{Store: store, Provider: paysim.Gateway{DB: db}, Log: log, ChargeTimeout: chargeTimeout}
	account := &usecase.Account{Ledger: store}

	bus.EnsureTopics(ctx, brokers, log, 3, events.TopicTitles, events.TopicOrders)
	dlq, err := bus.NewProducer(brokers)
	if err != nil {
		return err
	}
	defer dlq.Close()
	consumer := &bus.Consumer{Brokers: brokers, Group: "billing", Topics: []string{events.TopicTitles},
		Handle: account.HandleCatalog, DLQ: dlq, Log: log}

	prod, err := bus.NewProducer(brokers)
	if err != nil {
		return err
	}
	defer prod.Close()
	relay := &outbox.Relay{Pool: pools.Owner, Pub: prod, Log: log}

	gs := grpc.NewServer()
	billingv1.RegisterAccessServer(gs, &rpc.Server{Account: account})
	ln, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		return err
	}

	var wg sync.WaitGroup
	wg.Add(4)
	go func() { defer wg.Done(); relay.Run(ctx) }()
	go func() { defer wg.Done(); _ = gs.Serve(ln) }()
	go func() { defer wg.Done(); _ = consumer.Run(ctx) }()
	go func() { defer wg.Done(); saga.RunRecovery(ctx, 5*time.Second, 10*time.Second) }()

	health := &web.Health{Check: db.Ping}
	r := web.Secured(log, principal.Signer{Key: []byte(key)})
	health.Mount(r)
	(&rest.API{Saga: saga, Account: account}).Mount(r, web.Adapt(log))
	err = web.Serve(ctx, log, addr, r, health, 2*time.Second, 10*time.Second)
	stop()
	gs.GracefulStop()
	wg.Wait()
	return err
}
