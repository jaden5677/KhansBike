// Package http assembles the API's routing table and HTTP server. router.go is
// intentionally the single map of the API surface: reading it top to bottom
// should tell you every route the service exposes.
package http

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/khansbikezone/bikezone-api/internal/http/handler"
	mw "github.com/khansbikezone/bikezone-api/internal/http/middleware"
)

// RouterDeps carries the collaborators the routes need. It grows as checkpoints
// add services; keeping it a struct means main wires dependencies explicitly
// rather than reaching into globals.
type RouterDeps struct {
	Logger *slog.Logger
	DB     handler.Pinger // nil until the pool is wired
}

// NewRouter builds the chi router with the base middleware stack applied to
// every request, then mounts the current route set. The order of middleware is
// deliberate: request id first (so every later log line and panic carries it),
// then access logging, then panic recovery closest to the handler.
func NewRouter(deps RouterDeps) http.Handler {
	r := chi.NewRouter()

	r.Use(mw.RequestID)
	r.Use(mw.Logging(deps.Logger))
	r.Use(mw.Recoverer(deps.Logger))

	health := handler.NewHealth(deps.DB)
	r.Get("/healthz", health.Live)
	r.Get("/readyz", health.Ready)

	// TODO(phase-4): mount the public /api/v1 catalog, search, and media routes.
	// TODO(phase-5): mount the admin and auth routes behind RBAC middleware.

	return r
}
