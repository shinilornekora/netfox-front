// playback — старт просмотра, прогресс, история, «Смотреть позже».
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"plenka/contracts/events"
	"plenka/kit/bus"
	"plenka/kit/env"
	"plenka/kit/pgkit"
	"plenka/kit/principal"
	"plenka/kit/web"
	"plenka/playback/internal/adapter/access"
	"plenka/playback/internal/adapter/pgsql"
	"plenka/playback/internal/adapter/rest"
	"plenka/playback/internal/usecase"
	"plenka/playback/migrations"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "playback:", err)
		os.Exit(1)
	}
}

func run() error {
	var e env.Reader
	addr := e.Str("HTTP_ADDR", ":8080")
	dsn := e.Must("DATABASE_URL")
	appDSN := e.Str("APP_DATABASE_URL", "")
	key := e.Must("INTERNAL_KEY")
	streamKey := e.Must("STREAM_KEY")
	billingAddr := e.Str("BILLING_GRPC", "localhost:9092")
	brokers := e.List("KAFKA_BROKERS", "localhost:9092")
	media := e.Str("MEDIA_DIR", "./media")
	if err := e.Err(); err != nil {
		return err
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("svc", "playback")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Две роли: владелец схемы (миграции, обслуживание, outbox) и роль
	// данных для запросов — она не может снять RLS (TRADEOFFS 3.8).
	pools, err := pgkit.OpenPools(ctx, log, dsn, appDSN, 20, migrations.FS)
	if err != nil {
		return err
	}
	defer pools.Close()
	db := pools.App
	store := pgsql.Store{DB: db}
	maint := pgsql.Store{DB: pools.Owner} // DDL секций — только владельцу

	acc, err := access.Dial(billingAddr)
	if err != nil {
		return err
	}
	defer acc.Close()
	uc := &usecase.Service{Store: store, Access: acc, StreamKey: []byte(streamKey),
		HasMedia: func(name string) bool {
			st, err := os.Stat(filepath.Join(media, filepath.Base(name)))
			return err == nil && st.Mode().IsRegular() && st.Size() > 0
		}}

	bus.EnsureTopics(ctx, brokers, log, 3, events.TopicTitles, events.TopicOrders)
	dlq, err := bus.NewProducer(brokers)
	if err != nil {
		return err
	}
	defer dlq.Close()
	consumer := &bus.Consumer{Brokers: brokers, Group: "playback", Topics: []string{events.TopicTitles, events.TopicOrders},
		Handle: func(ctx context.Context, env events.Envelope) error {
			if env.Type == events.OrderStatusChanged {
				return uc.HandleOrders(ctx, env)
			}
			return uc.HandleCatalog(ctx, env)
		}, DLQ: dlq, Log: log}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); _ = consumer.Run(ctx) }()
	go func() { // секции истории: 3 месяца вперёд и удаление старше 13 месяцев, раз в сутки
		defer wg.Done()
		t := time.NewTicker(24 * time.Hour)
		defer t.Stop()
		for {
			if n, err := maint.EnsurePartitions(ctx); err != nil {
				log.Warn("partitions", "err", err)
			} else if n > 0 {
				log.Info("view partitions created", "count", n)
			}
			if n, err := maint.DropOldPartitions(ctx, pgsql.HistoryKeepMonths); err != nil {
				log.Warn("partition retention", "err", err)
			} else if n > 0 {
				log.Info("old view partitions dropped", "count", n)
			}
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()

	health := &web.Health{Check: db.Ping}
	r := web.Secured(log, principal.Signer{Key: []byte(key)})
	health.Mount(r)
	(&rest.API{UC: uc, MediaDir: media}).Mount(r, web.Adapt(log))
	err = web.Serve(ctx, log, addr, r, health, 2*time.Second, 10*time.Second)
	stop()
	wg.Wait()
	return err
}
