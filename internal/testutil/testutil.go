//go:build integration

// Package testutil builds real, isolated environments for integration tests.
// Every test gets its own freshly created and migrated database, dropped when
// the test ends, so tests can run in parallel and never see each other's
// data. Tests using it need a reachable Postgres: set TEST_DATABASE_URL (or
// DATABASE_URL) to a server where the user may create databases.
package testutil

import (
	"bytes"
	"context"
	"crypto/rand"
	"io"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/khansbikezone/bikezone-api/internal/app"
	"github.com/khansbikezone/bikezone-api/internal/config"
)

// serverURL is the Postgres server the test databases are created on.
func serverURL(t *testing.T) string {
	t.Helper()
	for _, k := range []string{"TEST_DATABASE_URL", "DATABASE_URL"} {
		if u := os.Getenv(k); u != "" {
			return u
		}
	}
	t.Skip("set TEST_DATABASE_URL (or DATABASE_URL) to run integration tests")
	return ""
}

// NewDatabase creates an empty database and returns its URL. It is dropped
// (disconnecting any leftover sessions) when the test ends.
func NewDatabase(t *testing.T) string {
	t.Helper()
	server := serverURL(t)
	ctx := context.Background()
	name := "bz_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:16]

	conn, err := pgx.Connect(ctx, server)
	if err != nil {
		t.Fatalf("connect to test server: %v", err)
	}
	defer func() { _ = conn.Close(ctx) }()
	if _, err := conn.Exec(ctx, "CREATE DATABASE "+name); err != nil { // name is generated, not user input
		t.Fatalf("create test database: %v", err)
	}
	t.Cleanup(func() {
		c, err := pgx.Connect(ctx, server)
		if err != nil {
			t.Logf("drop test database %s: %v", name, err)
			return
		}
		defer func() { _ = c.Close(ctx) }()
		if _, err := c.Exec(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)"); err != nil {
			t.Logf("drop test database %s: %v", name, err)
		}
	})

	u, err := url.Parse(server)
	if err != nil {
		t.Fatalf("parse server url: %v", err)
	}
	u.Path = "/" + name
	return u.String()
}

// Config is a valid configuration for a fresh database, with media stored in
// a temporary directory and email going to the log.
func Config(t *testing.T) *config.Config {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	return &config.Config{
		AppEnv:               "test",
		HTTPAddr:             "127.0.0.1:0",
		PublicBaseURL:        "http://bikezone.test",
		DatabaseURL:          NewDatabase(t),
		DBMaxConns:           5,
		SessionCookieName:    "bz_session",
		SessionTTL:           time.Hour,
		CSRFKey:              key,
		MediaBackend:         config.MediaBackendFS,
		MediaFSRoot:          t.TempDir(),
		MediaMaxUploadBytes:  10 << 20,
		MediaMaxPixels:       50_000_000,
		MediaRenditionWidths: []int{320, 640},
		EmailBackend:         config.EmailBackendLog,
		EmailFromName:        "Khan's Bike Zone",
		SMTPPort:             587,
		WorkerConcurrency:    1,
		// Like production behind the tunnel: CF-Connecting-IP from loopback
		// is the client address, which lets tests simulate distinct clients.
		TrustCloudflareIPs: true,
		LogLevel:           slog.LevelInfo,
		LogFormat:          "text",
	}
}

// LogBuffer is a goroutine-safe log sink. The log email backend writes the
// messages it would send to the log, so tests read confirmation links here.
type LogBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *LogBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *LogBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// App builds the whole application on a fresh, migrated database, logging to
// logs (pass nil to discard).
func App(t *testing.T, logs io.Writer) *app.App {
	t.Helper()
	if logs == nil {
		logs = io.Discard
	}
	ctx := context.Background()
	a, err := app.New(ctx, Config(t), slog.New(slog.NewTextHandler(logs, nil)))
	if err != nil {
		t.Fatalf("build app: %v", err)
	}
	t.Cleanup(a.Close)
	if err := a.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return a
}

// DrainJobs runs queued jobs until the queue is empty, as the worker would.
func DrainJobs(t *testing.T, a *app.App) {
	t.Helper()
	r := a.Runner()
	for i := 0; i < 1000; i++ {
		ran, err := r.ProcessNext(context.Background(), "test-worker")
		if err != nil {
			t.Fatalf("process job: %v", err)
		}
		if !ran {
			return
		}
	}
	t.Fatal("job queue did not drain")
}
