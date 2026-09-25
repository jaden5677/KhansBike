package handler

import (
	"log/slog"
	"net/http"

	"github.com/khansbikezone/bikezone-api/internal/http/dto"
	"github.com/khansbikezone/bikezone-api/internal/service"
)

// Products serves the admin product endpoints. Products are edited as whole
// documents; each response carries an ETag, and sending it back in If-Match
// makes a save fail with 412 if someone else saved in between (the web admin
// and the phone editing the same product).
type Products struct {
	base
	svc  *service.Products
	urls dto.URLFunc
}

// NewProducts builds the admin product handlers.
func NewProducts(log *slog.Logger, svc *service.Products, urls dto.URLFunc) *Products {
	return &Products{base: base{log: log}, svc: svc, urls: urls}
}

func (h *Products) writeProduct(w http.ResponseWriter, status int, v *service.ProductView) {
	w.Header().Set("ETag", service.ETag(v.Product.UpdatedAt))
	writeJSON(w, status, dto.NewAdminProduct(v, h.urls))
}

// List answers GET /admin/products: any status, same filters as the public
// listing plus status=, default order most recently edited first.
func (h *Products) List(w http.ResponseWriter, r *http.Request) {
	q, err := productQuery(r.URL.Query())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	page, err := h.svc.List(r.Context(), q)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, dto.NewAdminProductPage(page, h.urls))
}

// Get answers GET /admin/products/{id}.
func (h *Products) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := h.idParam(w, r, "id")
	if !ok {
		return
	}
	v, err := h.svc.Get(r.Context(), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.writeProduct(w, http.StatusOK, v)
}

// Create answers POST /admin/products.
func (h *Products) Create(w http.ResponseWriter, r *http.Request) {
	var in service.ProductInput
	if !h.decode(w, r, &in) {
		return
	}
	v, err := h.svc.Create(r.Context(), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.writeProduct(w, http.StatusCreated, v)
}

// Update answers PUT /admin/products/{id} (honours If-Match).
func (h *Products) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := h.idParam(w, r, "id")
	if !ok {
		return
	}
	var in service.ProductInput
	if !h.decode(w, r, &in) {
		return
	}
	v, err := h.svc.Update(r.Context(), id, in, r.Header.Get("If-Match"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.writeProduct(w, http.StatusOK, v)
}

// Delete answers DELETE /admin/products/{id}.
func (h *Products) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := h.idParam(w, r, "id")
	if !ok {
		return
	}
	if err := h.svc.Delete(r.Context(), id); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// AttachMedia answers POST /admin/products/{id}/media.
func (h *Products) AttachMedia(w http.ResponseWriter, r *http.Request) {
	id, ok := h.idParam(w, r, "id")
	if !ok {
		return
	}
	var in service.MediaInput
	if !h.decode(w, r, &in) {
		return
	}
	m, err := h.svc.AttachMedia(r.Context(), id, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, dto.NewAdminMedia(m, h.urls))
}

// UpdateMedia answers PUT /admin/products/{id}/media/{mediaId}.
func (h *Products) UpdateMedia(w http.ResponseWriter, r *http.Request) {
	id, ok := h.idParam(w, r, "id")
	if !ok {
		return
	}
	mediaID, ok := h.idParam(w, r, "mediaId")
	if !ok {
		return
	}
	var in service.MediaInput
	if !h.decode(w, r, &in) {
		return
	}
	m, err := h.svc.UpdateMedia(r.Context(), id, mediaID, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, dto.NewAdminMedia(m, h.urls))
}

// DetachMedia answers DELETE /admin/products/{id}/media/{mediaId}.
func (h *Products) DetachMedia(w http.ResponseWriter, r *http.Request) {
	id, ok := h.idParam(w, r, "id")
	if !ok {
		return
	}
	mediaID, ok := h.idParam(w, r, "mediaId")
	if !ok {
		return
	}
	if err := h.svc.DetachMedia(r.Context(), id, mediaID); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
