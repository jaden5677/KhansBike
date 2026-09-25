package handler

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/khansbikezone/bikezone-api/internal/http/dto"
	"github.com/khansbikezone/bikezone-api/internal/service"
)

// Catalog serves the public, read-only catalogue.
type Catalog struct {
	base
	svc  *service.Catalog
	urls dto.URLFunc
}

// NewCatalog builds the public catalogue handlers.
func NewCatalog(log *slog.Logger, svc *service.Catalog, urls dto.URLFunc) *Catalog {
	return &Catalog{base: base{log: log}, svc: svc, urls: urls}
}

// Categories answers GET /categories: the visible category tree.
func (h *Catalog) Categories(w http.ResponseWriter, r *http.Request) {
	tree, err := h.svc.CategoryTree(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": dto.NewCategoryNodes(tree.Roots, tree.Images, h.urls)})
}

// Category answers GET /categories/{slug}.
func (h *Catalog) Category(w http.ResponseWriter, r *http.Request) {
	d, err := h.svc.Category(r.Context(), chi.URLParam(r, "slug"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, dto.NewCategoryPage(d, h.urls))
}

// Facets answers GET /categories/{slug}/facets: the filter options for the
// category under the filters in the query string.
func (h *Catalog) Facets(w http.ResponseWriter, r *http.Request) {
	q, err := productQuery(r.URL.Query())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	q.CategorySlug = chi.URLParam(r, "slug")
	facets, err := h.svc.Facets(r.Context(), q)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": facets})
}

// Products answers GET /products: a filtered, paginated listing.
func (h *Catalog) Products(w http.ResponseWriter, r *http.Request) {
	q, err := productQuery(r.URL.Query())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	q.Status = "" // public listings are always active products only
	page, err := h.svc.ListProducts(r.Context(), q)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, dto.NewProductPage(page, h.urls))
}

// Product answers GET /products/{slug}.
func (h *Catalog) Product(w http.ResponseWriter, r *http.Request) {
	v, err := h.svc.Product(r.Context(), chi.URLParam(r, "slug"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, dto.NewProductDetail(v, h.urls))
}

// Search answers GET /search?q=: matches grouped by category.
func (h *Catalog) Search(w http.ResponseWriter, r *http.Request) {
	groups, err := h.svc.Search(r.Context(), r.URL.Query().Get("q"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"groups": dto.NewSearchGroups(groups, h.urls)})
}

// Suggest answers GET /search/suggest?q=: typeahead names.
func (h *Catalog) Suggest(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.Suggest(r.Context(), r.URL.Query().Get("q"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// Fitment answers GET /fitment/{wheelSize}: everything that fits a wheel size.
func (h *Catalog) Fitment(w http.ResponseWriter, r *http.Request) {
	size := chi.URLParam(r, "wheelSize")
	groups, err := h.svc.Fitment(r.Context(), size)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"wheelSize": size, "groups": dto.NewSearchGroups(groups, h.urls)})
}

// Brands answers GET /brands.
func (h *Catalog) Brands(w http.ResponseWriter, r *http.Request) {
	bl, err := h.svc.Brands(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": dto.NewPublicBrands(bl, h.urls)})
}
