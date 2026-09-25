// Command api is the HTTP server entrypoint for Khan's Bike Zone. It loads and
// validates configuration, connects to the database and applies any pending
// migrations, then serves the API (and the embedded web app) until it receives
// an interrupt, at which point it drains in-flight requests within a bounded
// deadline.
//
// This is the single service the Windows host runs; the background job worker
// is hosted in-process (WORKER_ENABLED) rather than as a separate daemon.
// cmd/worker runs the same worker on its own when that is preferred.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/khansbikezone/bikezone-api/internal/app"
	"github.com/khansbikezone/bikezone-api/internal/config"
	bzhttp "github.com/khansbikezone/bikezone-api/internal/http"
	"github.com/khansbikezone/bikezone-api/web"
)

func main() {
	if err := run(); err != nil {
		// The logger may not exist yet if config failed, so report to stderr and
		// exit non-zero. This is the only place the program exits on error.
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

// shutdownTimeout bounds how long in-flight requests get to finish.
const shutdownTimeout = 30 * time.Second

func run() error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	logger := config.NewLogger(cfg)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	a, err := app.New(ctx, cfg, logger)
	if err != nil {
		return err
	}
	defer a.Close()
	// The binary migrates its own database, so deploying is replacing the .exe.
	if err := a.Migrate(ctx); err != nil {
		return err
	}

	var workers sync.WaitGroup
	if cfg.WorkerEnabled {
		runner := a.Runner()
		workers.Add(1)
		go func() {
			defer workers.Done()
			runner.Run(ctx) // returns after ctx is cancelled and jobs have wound down
		}()
	}

	srv := bzhttp.NewServer(cfg, a.Router(web.Handler()))
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
	case err = <-serveErr: // the server could not start (e.g. the port is taken)
		stop()
	case <-ctx.Done():
		logger.Info("shutdown signal received, draining connections")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if serr := srv.Shutdown(shutdownCtx); serr != nil {
			err = fmt.Errorf("graceful shutdown failed: %w", serr)
		}
	}
	workers.Wait()
	if err == nil {
		logger.Info("server stopped cleanly")
	}
	return err
}
