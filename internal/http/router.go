// Package http assembles the API's routing table and HTTP server. router.go is
// intentionally the single map of the API surface: reading it top to bottom
// should tell you every route the service exposes.
package http

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/khansbikezone/bikezone-api/internal/auth"
	"github.com/khansbikezone/bikezone-api/internal/config"
	"github.com/khansbikezone/bikezone-api/internal/domain"
	"github.com/khansbikezone/bikezone-api/internal/http/handler"
	mw "github.com/khansbikezone/bikezone-api/internal/http/middleware"
	"github.com/khansbikezone/bikezone-api/internal/http/problem"
	"github.com/khansbikezone/bikezone-api/internal/importer"
	"github.com/khansbikezone/bikezone-api/internal/service"
)

// RouterDeps carries the collaborators the routes need. Keeping it a struct
// means main wires dependencies explicitly rather than reaching into globals.
type RouterDeps struct {
	Config *config.Config
	Logger *slog.Logger
	DB     handler.Pinger

	Auth        *auth.Service
	Catalog     *service.Catalog
	Taxonomy    *service.Taxonomy
	Products    *service.Products
	Media       *service.Media
	Subscribers *service.Subscribers
	Audit       *service.Audit
	Importer    *importer.Importer

	// Web serves the embedded front end for every path the API does not own.
	// Nil serves nothing there (404).
	Web http.Handler
}

// Cache policies: the anonymous catalogue may be cached briefly (by browsers
// and by Cloudflare); nothing authenticated may be cached anywhere.
const (
	publicCache = "public, max-age=60"
	noStore     = "no-store"
)

// NewRouter builds the chi router with the base middleware stack applied to
// every request, then mounts the route set. The order of middleware is
// deliberate: request id first (so every later log line and panic carries it),
// then the client address (for logs and rate limits), then access logging,
// then panic recovery closest to the handler.
func NewRouter(deps RouterDeps) http.Handler {
	cfg, log := deps.Config, deps.Logger
	urls := deps.Media.URL

	health := handler.NewHealth(deps.DB)
	catalog := handler.NewCatalog(log, deps.Catalog, urls)
	subscribers := handler.NewSubscribers(log, deps.Subscribers)
	authH := handler.NewAuth(log, deps.Auth, handler.CookieConfig{Name: cfg.SessionCookieName, Secure: cfg.IsProduction()}, cfg.PublicBaseURL)
	taxonomy := handler.NewTaxonomy(log, deps.Taxonomy)
	products := handler.NewProducts(log, deps.Products, urls)
	media := handler.NewMedia(log, deps.Media, urls, cfg.MediaMaxUploadBytes)
	imports := handler.NewImports(log, deps.Importer)
	audit := handler.NewAudit(log, deps.Audit)

	authenticated := mw.Authenticate(deps.Auth, cfg.SessionCookieName, log)

	r := chi.NewRouter()
	r.Use(mw.RequestID)
	r.Use(mw.ClientIP(cfg.TrustCloudflareIPs))
	r.Use(mw.Logging(log))
	r.Use(mw.Recoverer(log))
	r.Use(mw.SecureHeaders(cfg.IsProduction()))

	r.Get("/healthz", health.Live)
	r.Get("/readyz", health.Ready)

	// Rendition files (MEDIA_BACKEND=fs, or r2 without a public bucket URL).
	r.Get("/media/*", media.Serve)

	r.Route("/api/v1", func(r chi.Router) {
		r.NotFound(func(w http.ResponseWriter, _ *http.Request) {
			problem.Write(w, problem.New(http.StatusNotFound, "No such endpoint."))
		})
		r.MethodNotAllowed(func(w http.ResponseWriter, _ *http.Request) {
			problem.Write(w, problem.New(http.StatusMethodNotAllowed, "Method not allowed on this endpoint."))
		})

		// Public catalogue: anonymous and read-only.
		r.Group(func(r chi.Router) {
			r.Use(mw.CacheControl(publicCache))
			r.Get("/categories", catalog.Categories)
			r.Get("/categories/{slug}", catalog.Category)
			r.Get("/categories/{slug}/facets", catalog.Facets)
			r.Get("/products", catalog.Products)
			r.Get("/products/{slug}", catalog.Product)
			r.Get("/search", catalog.Search)
			r.Get("/search/suggest", catalog.Suggest)
			r.Get("/fitment/{wheelSize}", catalog.Fitment)
			r.Get("/brands", catalog.Brands)
		})

		// Mailing list: the only customer-facing writes.
		r.Group(func(r chi.Router) {
			r.Use(mw.CacheControl(noStore))
			r.With(mw.RateLimit(5, 3)).Post("/subscribers", subscribers.Subscribe)
			r.With(mw.RateLimit(20, 10)).Post("/subscribers/confirm", subscribers.Confirm)
			r.With(mw.RateLimit(20, 10)).Post("/subscribers/unsubscribe", subscribers.Unsubscribe)
		})

		r.Route("/auth", func(r chi.Router) {
			r.Use(mw.CacheControl(noStore))
			r.With(mw.RateLimit(10, 5)).Post("/login", authH.Login)
			r.With(mw.RateLimit(10, 5)).Post("/devices", authH.Pair) // a phone redeems a pairing code
			r.Group(func(r chi.Router) {
				r.Use(authenticated)
				r.Get("/session", authH.Session)
				r.Post("/logout", authH.Logout)
				r.With(mw.RequireSession).Put("/password", authH.ChangePassword)
			})
		})

		// Administration: the web admin (session + CSRF) and the paired phone
		// (bearer token) share every route below.
		r.Route("/admin", func(r chi.Router) {
			r.Use(mw.CacheControl(noStore))
			r.Use(authenticated)
			r.Use(mw.RequireRole(domain.RoleAdmin))

			r.Get("/categories", taxonomy.Categories)
			r.Post("/categories", taxonomy.CreateCategory)
			r.Get("/categories/{id}", taxonomy.Category)
			r.Put("/categories/{id}", taxonomy.UpdateCategory)
			r.Delete("/categories/{id}", taxonomy.DeleteCategory)
			r.Get("/categories/{id}/attributes", taxonomy.CategoryAttributes)
			r.Put("/categories/{id}/attributes/{attributeId}", taxonomy.BindAttribute)
			r.Delete("/categories/{id}/attributes/{attributeId}", taxonomy.UnbindAttribute)
			r.Get("/categories/{id}/form-schema", taxonomy.FormSchema)

			r.Get("/attributes", taxonomy.Attributes)
			r.Post("/attributes", taxonomy.CreateAttribute)
			r.Get("/attributes/{id}", taxonomy.Attribute)
			r.Put("/attributes/{id}", taxonomy.UpdateAttribute)
			r.Delete("/attributes/{id}", taxonomy.DeleteAttribute)
			r.Post("/attributes/{id}/options", taxonomy.CreateOption)
			r.Put("/attributes/{id}/options/{optionId}", taxonomy.UpdateOption)
			r.Delete("/attributes/{id}/options/{optionId}", taxonomy.DeleteOption)

			r.Get("/brands", taxonomy.Brands)
			r.Post("/brands", taxonomy.CreateBrand)
			r.Put("/brands/{id}", taxonomy.UpdateBrand)
			r.Delete("/brands/{id}", taxonomy.DeleteBrand)

			r.Get("/suppliers", taxonomy.Suppliers)
			r.Post("/suppliers", taxonomy.CreateSupplier)
			r.Put("/suppliers/{id}", taxonomy.UpdateSupplier)
			r.Delete("/suppliers/{id}", taxonomy.DeleteSupplier)

			r.Get("/products", products.List)
			r.Post("/products", products.Create)
			r.Get("/products/{id}", products.Get)
			r.Put("/products/{id}", products.Update)
			r.Delete("/products/{id}", products.Delete)
			r.Post("/products/{id}/media", products.AttachMedia)
			r.Put("/products/{id}/media/{mediaId}", products.UpdateMedia)
			r.Delete("/products/{id}/media/{mediaId}", products.DetachMedia)

			r.Post("/media", media.Upload)
			r.Get("/media/{id}", media.Asset)

			r.Get("/subscribers/stats", subscribers.Stats)
			r.Get("/subscribers/export", subscribers.Export)

			r.Get("/imports", imports.Batches)
			r.Post("/imports", imports.Stage)
			r.Get("/imports/{id}", imports.Batch)
			r.Get("/imports/{id}/rows", imports.Rows)
			r.Put("/imports/{id}/rows/{rowId}", imports.Decide)
			r.Post("/imports/{id}/commit", imports.Commit)
			r.Post("/imports/{id}/abort", imports.Abort)

			r.Get("/audit", audit.List)

			// Device management is reserved for the browser session, so a lost
			// phone cannot mint credentials for other phones.
			r.Group(func(r chi.Router) {
				r.Use(mw.RequireSession)
				r.Post("/devices/pairing-codes", authH.CreatePairingCode)
				r.Get("/devices", authH.Devices)
				r.Delete("/devices/{id}", authH.RevokeDevice)
			})
		})
	})

	if deps.Web != nil {
		r.NotFound(deps.Web.ServeHTTP)
	}
	return r
}
