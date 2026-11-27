// identity — аккаунты зрителей и серверные сессии.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"

	identityv1 "plenka/contracts/gen/identity/v1"
	"plenka/identity/internal/adapter/pgsql"
	"plenka/identity/internal/adapter/rest"
	"plenka/identity/internal/adapter/rpc"
	"plenka/identity/internal/core"
	"plenka/identity/internal/usecase"
	"plenka/identity/migrations"
	"plenka/kit/env"
	"plenka/kit/pgkit"
	"plenka/kit/principal"
	"plenka/kit/web"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "identity:", err)
		os.Exit(1)
	}
}

func run() error {
	var e env.Reader
	httpAddr := e.Str("HTTP_ADDR", ":8080")
	grpcAddr := e.Str("GRPC_ADDR", ":9090")
	dsn := e.Must("DATABASE_URL")
	appDSN := e.Str("APP_DATABASE_URL", "")
	key := e.Must("INTERNAL_KEY")
	adminEmail, adminPw := e.Str("ADMIN_EMAIL", ""), e.Str("ADMIN_PASSWORD", "")
	argonMem := e.Int("ARGON_MEMORY_KIB", 19*1024)
	secure := e.Str("COOKIE_SECURE", "false") == "true"
	if err := e.Err(); err != nil {
		return err
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("svc", "identity")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Две роли: владелец схемы (миграции, обслуживание, outbox) и роль
	// данных для запросов — она не может снять RLS (TRADEOFFS 3.8).
	pools, err := pgkit.OpenPools(ctx, log, dsn, appDSN, 10, migrations.FS)
	if err != nil {
		return err
	}
	defer pools.Close()
	db := pools.App

	uc := &usecase.Service{
		Accounts: pgsql.Accounts{DB: db},
		Sessions: pgsql.Sessions{DB: db},
		Hasher:   pgsql.Argon2{MemoryKiB: uint32(argonMem)},
		Policy:   core.DefaultPolicy(),
	}
	if created, err := uc.EnsureAdmin(ctx, adminEmail, adminPw); err != nil {
		return err
	} else if created {
		log.Info("admin created", "email", adminEmail)
	}

	gs := grpc.NewServer()
	identityv1.RegisterSessionsServer(gs, &rpc.Server{UC: uc})
	ln, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		return err
	}
	go func() { _ = gs.Serve(ln) }()
	defer gs.GracefulStop()

	health := &web.Health{Check: db.Ping}
	r := web.Secured(log, principal.Signer{Key: []byte(key)})
	health.Mount(r)
	(&rest.API{UC: uc, SecureCookie: secure}).Mount(r, web.Adapt(log))
	return web.Serve(ctx, log, httpAddr, r, health, 2*time.Second, 10*time.Second)
}
