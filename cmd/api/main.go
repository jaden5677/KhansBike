// Command api is the HTTP server entrypoint for Khan's Bike Zone. It loads and
// validates configuration, builds the structured logger, assembles the router,
// and serves until it receives an interrupt, at which point it drains in-flight
// requests within a bounded deadline.
//
// This is the single service the Windows host runs; the background job worker is
// hosted in-process (WORKER_ENABLED) rather than as a separate daemon, added in
// a later checkpoint.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/khansbikezone/bikezone-api/internal/config"
	bzhttp "github.com/khansbikezone/bikezone-api/internal/http"
)

func main() {
	if err := run(); err != nil {
		// The logger may not exist yet if config failed, so report to stderr and
		// exit non-zero. This is the only place the program exits on error.
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	logger := config.NewLogger(cfg)

	// TODO(phase-3): construct the pgxpool here and pass a Pinger into RouterDeps
	// so /readyz reflects real database health; nil for now keeps the skeleton
	// bootable without a database.
	router := bzhttp.NewRouter(bzhttp.RouterDeps{Logger: logger, DB: nil})
	srv := bzhttp.NewServer(cfg, router)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Serve in a goroutine so the main goroutine can wait for a shutdown signal.
	serveErr := make(chan error, 1)
	go func() {
		logger.Info("http server starting", "addr", cfg.HTTPAddr, "env", cfg.AppEnv)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
			return
		}
		serveErr <- nil
	}()

	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
		logger.Info("shutdown signal received, draining connections")
	}

	// Give in-flight requests up to 30s to complete before forcing the close.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown failed: %w", err)
	}
	logger.Info("server stopped cleanly")
	return nil
}
