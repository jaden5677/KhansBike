// Command worker runs the background job worker on its own: search
// reindexing, image processing and confirmation emails. The API normally hosts
// the same worker in-process (WORKER_ENABLED=true); run this binary instead
// (with WORKER_ENABLED=false on the API) to process jobs in a separate process.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/khansbikezone/bikezone-api/internal/app"
	"github.com/khansbikezone/bikezone-api/internal/config"
)

func main() {
	if err := run(); err != nil {
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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	a, err := app.New(ctx, cfg, logger)
	if err != nil {
		return err
	}
	defer a.Close()
	if err := a.Migrate(ctx); err != nil { // safe alongside the API: migrations take a lock
		return err
	}
	a.Runner().Run(ctx)
	return nil
}
