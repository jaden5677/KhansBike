// Package app assembles the application from configuration: the database
// pool, the store, every service, the job runner and the HTTP router. All the
// binaries (api, worker, importer, admin) start from here, so the wiring
// exists in exactly one place.
package app

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"net/mail"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"

	"github.com/khansbikezone/bikezone-api/db"
	"github.com/khansbikezone/bikezone-api/internal/auth"
	"github.com/khansbikezone/bikezone-api/internal/config"
	"github.com/khansbikezone/bikezone-api/internal/email"
	bzhttp "github.com/khansbikezone/bikezone-api/internal/http"
	"github.com/khansbikezone/bikezone-api/internal/importer"
	"github.com/khansbikezone/bikezone-api/internal/jobs"
	"github.com/khansbikezone/bikezone-api/internal/media"
	"github.com/khansbikezone/bikezone-api/internal/service"
	"github.com/khansbikezone/bikezone-api/internal/store"
)

// App holds the constructed application.
type App struct {
	Config *config.Config
	Logger *slog.Logger
	Store  *store.Store

	Auth        *auth.Service
	Catalog     *service.Catalog
	Taxonomy    *service.Taxonomy
	Products    *service.Products
	Media       *service.Media
	Subscribers *service.Subscribers
	Audit       *service.Audit
	Importer    *importer.Importer

	pool *pgxpool.Pool
}

// connectTimeout bounds the initial database connection at startup.
const connectTimeout = 10 * time.Second

// New connects to the database and builds every service. Call Close when done.
func New(ctx context.Context, cfg *config.Config, logger *slog.Logger) (*App, error) {
	pool, err := connect(ctx, cfg)
	if err != nil {
		return nil, err
	}
	blobs, mediaURL, err := blobStore(cfg)
	if err != nil {
		pool.Close()
		return nil, err
	}
	st := store.New(pool)
	taxonomy := service.NewTaxonomy(st)
	products := service.NewProducts(st)
	return &App{
		Config:   cfg,
		Logger:   logger,
		Store:    st,
		Auth:     auth.NewService(st, cfg.CSRFKey, cfg.SessionTTL, logger),
		Catalog:  service.NewCatalog(st),
		Taxonomy: taxonomy,
		Products: products,
		Media: service.NewMedia(st, blobs, service.MediaConfig{
			MaxUploadBytes: cfg.MediaMaxUploadBytes,
			MaxPixels:      cfg.MediaMaxPixels,
			Widths:         cfg.MediaRenditionWidths,
			PublicBaseURL:  mediaURL,
		}),
		Subscribers: service.NewSubscribers(st, emailSender(cfg, logger), cfg.PublicBaseURL),
		Audit:       service.NewAudit(st),
		Importer:    importer.New(st, taxonomy, products),
		pool:        pool,
	}, nil
}

func connect(ctx context.Context, cfg *config.Config) (*pgxpool.Pool, error) {
	pcfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	pcfg.MaxConns = cfg.DBMaxConns
	ctx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(ctx, pcfg)
	if err != nil {
		return nil, fmt.Errorf("connect to database: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("connect to database: %w", err)
	}
	return pool, nil
}

// blobStore picks the media backend and the base URL renditions are served
// from: the R2 bucket's public domain when one is configured, otherwise this
// server's own /media route.
func blobStore(cfg *config.Config) (media.BlobStore, string, error) {
	local := cfg.PublicBaseURL + "/media"
	if cfg.MediaBackend == config.MediaBackendR2 {
		s, err := media.NewR2Store(cfg.R2AccountID, cfg.R2Bucket, cfg.R2AccessKeyID, cfg.R2SecretAccessKey)
		if err != nil {
			return nil, "", err
		}
		if cfg.R2PublicBase != "" {
			return s, cfg.R2PublicBase, nil
		}
		return s, local, nil
	}
	s, err := media.NewFSStore(cfg.MediaFSRoot)
	if err != nil {
		return nil, "", err
	}
	return s, local, nil
}

func emailSender(cfg *config.Config, logger *slog.Logger) email.Sender {
	if cfg.EmailBackend == config.EmailBackendSMTP {
		return email.SMTPSender{
			Host:     cfg.SMTPHost,
			Port:     cfg.SMTPPort,
			Username: cfg.SMTPUsername,
			Password: cfg.SMTPPassword,
			From:     mail.Address{Name: cfg.EmailFromName, Address: cfg.EmailFrom},
		}
	}
	return email.LogSender{Logger: logger}
}

// Close releases the database pool.
func (a *App) Close() { a.pool.Close() }

// Migrate applies any pending migrations embedded in the binary. A Postgres
// advisory lock serialises concurrent callers, so the API and the worker can
// both start (and both migrate) safely at the same moment.
func (a *App) Migrate(ctx context.Context) error {
	migrations, err := fs.Sub(db.Migrations, db.MigrationsDir)
	if err != nil {
		return fmt.Errorf("load migrations: %w", err)
	}
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return fmt.Errorf("migration lock: %w", err)
	}
	sqlDB := stdlib.OpenDBFromPool(a.pool)
	defer func() { _ = sqlDB.Close() }()
	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, migrations, goose.WithSessionLocker(locker))
	if err != nil {
		return fmt.Errorf("migrations: %w", err)
	}
	results, err := provider.Up(ctx)
	if err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	for _, r := range results {
		a.Logger.Info("migration applied", "version", r.Source.Version, "file", r.Source.Path, "duration", r.Duration)
	}
	return nil
}

// Router builds the HTTP handler; web serves the embedded front end.
func (a *App) Router(web http.Handler) http.Handler {
	return bzhttp.NewRouter(bzhttp.RouterDeps{
		Config:      a.Config,
		Logger:      a.Logger,
		DB:          a.Store,
		Auth:        a.Auth,
		Catalog:     a.Catalog,
		Taxonomy:    a.Taxonomy,
		Products:    a.Products,
		Media:       a.Media,
		Subscribers: a.Subscribers,
		Audit:       a.Audit,
		Importer:    a.Importer,
		Web:         web,
	})
}

// Runner builds the background job runner with every job kind registered.
func (a *App) Runner() *jobs.Runner {
	r := jobs.NewRunner(a.Store, a.Logger, a.Config.WorkerConcurrency)
	r.Handle(service.JobReindexSearch, a.Catalog.ReindexSearch)
	r.Handle(service.JobProcessMedia, a.Media.ProcessJob)
	r.Handle(service.JobSendConfirmation, a.Subscribers.SendConfirmationJob)
	return r
}
