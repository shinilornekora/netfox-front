// gateway — единая точка входа /api/* для SPA.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	identityv1 "plenka/contracts/gen/identity/v1"
	"plenka/gateway/internal/edge"
	"plenka/kit/env"
	"plenka/kit/principal"
	"plenka/kit/web"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "gateway:", err)
		os.Exit(1)
	}
}

func run() error {
	var e env.Reader
	addr := e.Str("HTTP_ADDR", ":8080")
	key := e.Must("INTERNAL_KEY")
	idGRPC := e.Str("IDENTITY_GRPC", "localhost:9091")
	ups := map[string]string{
		"identity": e.Str("IDENTITY_URL", "http://localhost:8081"),
		"catalog":  e.Str("CATALOG_URL", "http://localhost:8082"),
		"billing":  e.Str("BILLING_URL", "http://localhost:8083"),
		"playback": e.Str("PLAYBACK_URL", "http://localhost:8084"),
	}
	if err := e.Err(); err != nil {
		return err
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("svc", "gateway")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	conn, err := grpc.NewClient(idGRPC, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	defer conn.Close()

	mk := func(name string, timeout time.Duration, slots int, prefixes ...string) *edge.Upstream {
		u, _ := url.Parse(ups[name])
		return &edge.Upstream{Name: name, URL: u, Timeout: timeout, Slots: slots, Prefixes: prefixes}
	}
	g := &edge.Edge{
		Log:    log,
		Signer: principal.Signer{Key: []byte(key)},
		Sessions: &edge.Sessions{RPC: identityv1.NewSessionsClient(conn),
			PosTTL: 20 * time.Second, NegTTL: 5 * time.Second, Budget: 500 * time.Millisecond},
		Upstreams: []*edge.Upstream{
			mk("identity", 5*time.Second, 64, "/api/auth", "/api/me"),
			mk("catalog", 5*time.Second, 128, "/api/titles", "/api/genres", "/api/admin/titles"),
			mk("billing", 10*time.Second, 48, "/api/billing"),
			mk("playback", 5*time.Second, 128, "/api/playback"),
			// Поток видео: без общего тайм-аута (фильм длится часы) и со
			// своим отсеком, чтобы долгие загрузки не заняли слоты API.
			mk("playback", 0, 256, "/api/playback/stream"),
		},
		PerUser:   &edge.Window{Limit: 300, Size: time.Minute},
		AuthLimit: &edge.Window{Limit: 10, Size: time.Minute},
		TrustXFF:  e.Str("TRUST_XFF", "false") == "true",
	}
	g.Init()

	health := &web.Health{}
	r := web.Base(log)
	health.Mount(r)
	r.Handle("/api/*", g)
	return web.Serve(ctx, log, addr, r, health, time.Second, 10*time.Second)
}
