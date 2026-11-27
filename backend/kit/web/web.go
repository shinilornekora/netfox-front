// Package web — HTTP-обвязка сервисов на chi.
package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"plenka/kit/principal"
	"plenka/kit/problem"
)

// Handler — обработчик, возвращающий ошибку: один путь ошибок на сервис,
// без повторяющегося «if err != nil { writeError; return }».
type Handler func(w http.ResponseWriter, r *http.Request) error

// Adapt превращает Handler в http.HandlerFunc.
func Adapt(log *slog.Logger) func(Handler) http.HandlerFunc {
	return func(h Handler) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			err := h(w, r)
			if err == nil {
				return
			}
			p := problem.From(err)
			if p.Status >= 500 {
				log.ErrorContext(r.Context(), "request failed", "err", err, "path", r.URL.Path)
			}
			cp := *p
			cp.Trace = middleware.GetReqID(r.Context())
			problem.Write(w, &cp)
		}
	}
}

// JSON пишет ответ.
func JSON(w http.ResponseWriter, status int, v any) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v == nil {
		return nil
	}
	return json.NewEncoder(w).Encode(v)
}

// NoContent — 204.
func NoContent(w http.ResponseWriter) error { w.WriteHeader(http.StatusNoContent); return nil }

// Bind читает JSON (до 256 КБ, неизвестные поля запрещены).
func Bind(r *http.Request, dst any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 256<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			return problem.New(http.StatusBadRequest, "empty_body", "Пустой запрос")
		}
		return problem.New(http.StatusBadRequest, "bad_json", "Некорректный JSON").With(err)
	}
	return nil
}

// Base — общий набор middleware: request id, реальный IP, журнал, восстановление.
func Base(log *slog.Logger) chi.Router {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, accessLog(log), middleware.Recoverer)
	return r
}

func accessLog(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/health") {
				next.ServeHTTP(w, r)
				return
			}
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			start := time.Now()
			next.ServeHTTP(ww, r)
			log.InfoContext(r.Context(), "http",
				"method", r.Method, "path", r.URL.Path, "status", ww.Status(),
				"ms", time.Since(start).Milliseconds(), "req", middleware.GetReqID(r.Context()))
		})
	}
}

// Health — /health/live и /health/ready. ready отключается при остановке.
type Health struct {
	draining atomic.Bool
	Check    func(context.Context) error
}

func (h *Health) Mount(r chi.Router) {
	r.Get("/health/live", func(w http.ResponseWriter, _ *http.Request) { _ = JSON(w, 200, map[string]string{"status": "up"}) })
	r.Get("/health/ready", func(w http.ResponseWriter, r *http.Request) {
		if h.draining.Load() {
			_ = JSON(w, 503, map[string]string{"status": "draining"})
			return
		}
		if h.Check != nil {
			ctx, cancel := context.WithTimeout(r.Context(), time.Second)
			defer cancel()
			if err := h.Check(ctx); err != nil {
				_ = JSON(w, 503, map[string]string{"status": "degraded"})
				return
			}
		}
		_ = JSON(w, 200, map[string]string{"status": "ready"})
	})
}

// Serve запускает сервер; при отмене ctx: ready=503, пауза drain, Shutdown.
func Serve(ctx context.Context, log *slog.Logger, addr string, h http.Handler, health *Health, drain, grace time.Duration) error {
	srv := &http.Server{Addr: addr, Handler: h, ReadHeaderTimeout: 3 * time.Second, IdleTimeout: 90 * time.Second}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	log.Info("listening", "addr", addr)
	select {
	case err := <-errc:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}
	if health != nil {
		health.draining.Store(true)
	}
	// Даём балансировщику заметить 503 на /health/ready, прежде чем
	// перестать принимать новые соединения.
	time.Sleep(drain)
	sctx, cancel := context.WithTimeout(context.Background(), grace)
	defer cancel()
	if err := srv.Shutdown(sctx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	log.Info("stopped")
	return nil
}

// Secured — роутер сервиса за gateway: проверка подписи заголовков личности.
func Secured(log *slog.Logger, signer principal.Signer) chi.Router {
	r := Base(log)
	r.Use(signer.Middleware)
	return r
}
