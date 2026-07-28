// Command migrate applies, rolls back, and inspects database migrations using
// the goose files embedded in the binary. It is a thin CLI over goose so the
// same migration set that cmd/api can self-apply is also drivable by hand during
// development and deployment.
//
// Usage:
//
//	migrate up                 # apply all pending migrations
//	migrate down               # roll back the most recent migration
//	migrate status             # show applied/pending state
//	migrate create <name> sql  # scaffold a new timestamped migration
//
// DATABASE_URL must be set in the environment.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver
	"github.com/pressly/goose/v3"

	"github.com/khansbikezone/bikezone-api/db"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		os.Exit(1)
	}
}

func run() error {
	args := os.Args[1:]
	if len(args) == 0 {
		return fmt.Errorf("usage: migrate <up|down|status|create> [args]")
	}
	command, cmdArgs := args[0], args[1:]

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}

	// goose requires a *sql.DB. This is the ONE place the project uses
	// database/sql: migration tooling needs it, and pgx's stdlib adapter lets us
	// keep a single driver. The application data path uses pgxpool exclusively.
	sqldb, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer func() { _ = sqldb.Close() }()

	goose.SetBaseFS(db.Migrations)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("set dialect: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := goose.RunContext(ctx, command, sqldb, db.MigrationsDir, cmdArgs...); err != nil {
		return fmt.Errorf("goose %s: %w", command, err)
	}
	return nil
}
