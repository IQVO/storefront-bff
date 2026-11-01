// Command api is the storefront BFF: the only storefront edge (/api/storefront).
//
// Environment (beyond the upstream URLs and budgets internal/config owns):
//
//	LOG_LEVEL                   debug | info | warn | error (default info)
//	OTEL_SERVICE_NAME           service.name of spans and metrics (default storefront-bff)
//	OTEL_EXPORTER_OTLP_ENDPOINT Collector OTLP/gRPC endpoint (default telemetry.DefaultEndpoint); export never blocks
//	SERVICE_VERSION             service.version (default dev)
//	ENVIRONMENT                 deployment.environment.name (default local)
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	inbound "github.com/IQVO/storefront-bff/internal/adapters/inbound/http"
	"github.com/IQVO/storefront-bff/internal/adapters/outbound/rest"
	"github.com/IQVO/storefront-bff/internal/adapters/outbound/telemetry"
	"github.com/IQVO/storefront-bff/internal/application/usecases"
	"github.com/IQVO/storefront-bff/internal/config"
)

func main() {
	log := newLogger(os.Getenv("LOG_LEVEL"))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, os.Getenv, log, nil)
	stop()
	if err != nil {
		log.Error("storefront-bff stopped", "error", err.Error())
		os.Exit(1)
	}
}

// newLogger builds the JSON logger; LOG_LEVEL debug|info|warn|warning|error
// (case-insensitive; "warning" is accepted like "warn" fleet-wide). The handler
// is wrapped so a record written with a span-carrying context also carries
// trace_id and span_id.
func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn", "warning":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	return slog.New(telemetry.WithTraceContext(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl})))
}

// run boots the edge and blocks until ctx ends. ready, when set, receives the
// listen address once the server accepts connections.
func run(ctx context.Context, getenv func(string) string, log *slog.Logger, ready chan<- string) error {
	slog.SetDefault(log)
	cfg, err := config.Load(getenv)
	if err != nil {
		return err
	}
	// Telemetry comes up before the router is built (otelchimetric binds the
	// global MeterProvider at construction) and is flushed LAST.
	tel := loadTelemetryEnv(getenv)
	flushTelemetry, err := startTelemetry(log, tel)
	if err != nil {
		return err
	}
	defer flushTelemetry()
	up, err := rest.New(cfg.Upstreams)
	if err != nil {
		return err
	}
	gw := usecases.NewGateway(up, cfg.Budgets, time.Now)
	srv := &http.Server{
		Handler:           inbound.NewServer(gw, log, cfg.CookieSecure, tel.serviceName).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	ln, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	log.Info("storefront-bff listening", "addr", ln.Addr().String())
	if ready != nil {
		ready <- ln.Addr().String()
	}
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdown); err != nil {
		return err
	}
	if err := <-errc; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
