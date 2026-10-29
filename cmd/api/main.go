// Command api is the storefront BFF: the only storefront edge (/api/storefront).
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
	"syscall"
	"time"

	inbound "github.com/IQVO/storefront-bff/internal/adapters/inbound/http"
	"github.com/IQVO/storefront-bff/internal/adapters/outbound/rest"
	"github.com/IQVO/storefront-bff/internal/application/usecases"
	"github.com/IQVO/storefront-bff/internal/config"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, os.Getenv, log, nil)
	stop()
	if err != nil {
		log.Error("storefront-bff stopped", "error", err.Error())
		os.Exit(1)
	}
}

// run boots the edge and blocks until ctx ends. ready, when set, receives the
// listen address once the server accepts connections.
func run(ctx context.Context, getenv func(string) string, log *slog.Logger, ready chan<- string) error {
	cfg, err := config.Load(getenv)
	if err != nil {
		return err
	}
	up, err := rest.New(cfg.Upstreams)
	if err != nil {
		return err
	}
	gw := usecases.NewGateway(up, cfg.Budgets, time.Now)
	srv := &http.Server{
		Handler:           inbound.NewServer(gw, log, cfg.CookieSecure).Handler(),
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
