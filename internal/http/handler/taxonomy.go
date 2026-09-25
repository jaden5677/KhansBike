package handler

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/khansbikezone/bikezone-api/internal/domain"
	"github.com/khansbikezone/bikezone-api/internal/http/dto"
	"github.com/khansbikezone/bikezone-api/internal/service"
)

// Taxonomy serves the admin endpoints for categories, attributes and their
// options, category attribute bindings, form schemas, brands and suppliers.
type Taxonomy struct {
	base
	svc *service.Taxonomy
}

// NewTaxonomy builds the taxonomy handlers.
func NewTaxonomy(log *slog.Logger, svc *service.Taxonomy) *Taxonomy {
	return &Taxonomy{base: base{log: log}, svc: svc}
}

// ---- categories ------------------------------------------------------------------

// Categories answers GET /admin/categories: every category, in tree order.
func (h *Taxonomy) Categories(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.ListCategories(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := make([]dto.Category, len(list))
	for i, c := range list {
		out[i] = dto.NewCategory(c)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

func writeCategory(w http.ResponseWriter, status int, c domain.Category) {
	w.Header().Set("ETag", service.ETag(c.UpdatedAt))
	writeJSON(w, status, dto.NewCategory(c))
}

// Category answers GET /admin/categories/{id}.
func (h *Taxonomy) Category(w http.ResponseWriter, r *http.Request) {
	id, ok := h.idParam(w, r, "id")
	if !ok {
		return
	}
	c, err := h.svc.GetCategory(r.Context(), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeCategory(w, http.StatusOK, c)
}

// CreateCategory answers POST /admin/categories.
func (h *Taxonomy) CreateCategory(w http.ResponseWriter, r *http.Request) {
	var in service.CategoryInput
	if !h.decode(w, r, &in) {
		return
	}
	c, err := h.svc.CreateCategory(r.Context(), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeCategory(w, http.StatusCreated, c)
}

// UpdateCategory answers PUT /admin/categories/{id} (honours If-Match).
func (h *Taxonomy) UpdateCategory(w http.ResponseWriter, r *http.Request) {
	id, ok := h.idParam(w, r, "id")
	if !ok {
		return
	}
	var in service.CategoryInput
	if !h.decode(w, r, &in) {
		return
	}
	c, err := h.svc.UpdateCategory(r.Context(), id, in, r.Header.Get("If-Match"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeCategory(w, http.StatusOK, c)
}

// DeleteCategory answers DELETE /admin/categories/{id}.
func (h *Taxonomy) DeleteCategory(w http.ResponseWriter, r *http.Request) {
	id, ok := h.idParam(w, r, "id")
	if !ok {
		return
	}
	if err := h.svc.DeleteCategory(r.Context(), id); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// CategoryAttributes answers GET /admin/categories/{id}/attributes.
func (h *Taxonomy) CategoryAttributes(w http.ResponseWriter, r *http.Request) {
	id, ok := h.idParam(w, r, "id")
	if !ok {
		return
	}
	bs, err := h.svc.CategoryAttributes(r.Context(), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": dto.NewBindings(bs)})
}

// BindAttribute answers PUT /admin/categories/{id}/attributes/{attributeId}.
func (h *Taxonomy) BindAttribute(w http.ResponseWriter, r *http.Request) {
	categoryID, ok := h.idParam(w, r, "id")
	if !ok {
		return
	}
	attributeID, ok := h.idParam(w, r, "attributeId")
	if !ok {
		return
	}
	var in service.BindingInput
	if !h.decode(w, r, &in) {
		return
	}
	if err := h.svc.BindAttribute(r.Context(), categoryID, attributeID, in); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// UnbindAttribute answers DELETE /admin/categories/{id}/attributes/{attributeId}.
// Products in the category lose their values for the attribute.
func (h *Taxonomy) UnbindAttribute(w http.ResponseWriter, r *http.Request) {
	categoryID, ok := h.idParam(w, r, "id")
	if !ok {
		return
	}
	attributeID, ok := h.idParam(w, r, "attributeId")
	if !ok {
		return
	}
	if err := h.svc.UnbindAttribute(r.Context(), categoryID, attributeID); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// FormSchema answers GET /admin/categories/{id}/form-schema. Clients cache it
// by version: the ETag lets them revalidate with If-None-Match for free.
func (h *Taxonomy) FormSchema(w http.ResponseWriter, r *http.Request) {
	id, ok := h.idParam(w, r, "id")
	if !ok {
		return
	}
	fs, err := h.svc.FormSchema(r.Context(), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	etag := `"` + strconv.Itoa(fs.Version) + `"`
	w.Header().Set("ETag", etag)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	writeJSON(w, http.StatusOK, fs)
}

// ---- attributes -------------------------------------------------------------------

func writeAttribute(w http.ResponseWriter, status int, a domain.Attribute) {
	w.Header().Set("ETag", service.ETag(a.UpdatedAt))
	writeJSON(w, status, dto.NewAttribute(a))
}

// Attributes answers GET /admin/attributes.
func (h *Taxonomy) Attributes(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.ListAttributes(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := make([]dto.Attribute, len(list))
	for i, a := range list {
		out[i] = dto.NewAttribute(a)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

// Attribute answers GET /admin/attributes/{id}.
func (h *Taxonomy) Attribute(w http.ResponseWriter, r *http.Request) {
	id, ok := h.idParam(w, r, "id")
	if !ok {
		return
	}
	a, err := h.svc.GetAttribute(r.Context(), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeAttribute(w, http.StatusOK, a)
}

// CreateAttribute answers POST /admin/attributes.
func (h *Taxonomy) CreateAttribute(w http.ResponseWriter, r *http.Request) {
	var in service.AttributeInput
	if !h.decode(w, r, &in) {
		return
	}
	a, err := h.svc.CreateAttribute(r.Context(), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeAttribute(w, http.StatusCreated, a)
}

// UpdateAttribute answers PUT /admin/attributes/{id} (honours If-Match).
func (h *Taxonomy) UpdateAttribute(w http.ResponseWriter, r *http.Request) {
	id, ok := h.idParam(w, r, "id")
	if !ok {
		return
	}
	var in service.AttributeInput
	if !h.decode(w, r, &in) {
		return
	}
	a, err := h.svc.UpdateAttribute(r.Context(), id, in, r.Header.Get("If-Match"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeAttribute(w, http.StatusOK, a)
}

// DeleteAttribute answers DELETE /admin/attributes/{id}.
func (h *Taxonomy) DeleteAttribute(w http.ResponseWriter, r *http.Request) {
	id, ok := h.idParam(w, r, "id")
	if !ok {
		return
	}
	if err := h.svc.DeleteAttribute(r.Context(), id); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// CreateOption answers POST /admin/attributes/{id}/options.
func (h *Taxonomy) CreateOption(w http.ResponseWriter, r *http.Request) {
	id, ok := h.idParam(w, r, "id")
	if !ok {
		return
	}
	var in service.OptionInput
	if !h.decode(w, r, &in) {
		return
	}
	o, err := h.svc.CreateOption(r.Context(), id, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, dto.NewOption(o))
}

// UpdateOption answers PUT /admin/attributes/{id}/options/{optionId}.
func (h *Taxonomy) UpdateOption(w http.ResponseWriter, r *http.Request) {
	id, ok := h.idParam(w, r, "id")
	if !ok {
		return
	}
	optionID, ok := h.idParam(w, r, "optionId")
	if !ok {
		return
	}
	var in service.OptionInput
	if !h.decode(w, r, &in) {
		return
	}
	o, err := h.svc.UpdateOption(r.Context(), id, optionID, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, dto.NewOption(o))
}

// DeleteOption answers DELETE /admin/attributes/{id}/options/{optionId}.
func (h *Taxonomy) DeleteOption(w http.ResponseWriter, r *http.Request) {
	id, ok := h.idParam(w, r, "id")
	if !ok {
		return
	}
	optionID, ok := h.idParam(w, r, "optionId")
	if !ok {
		return
	}
	if err := h.svc.DeleteOption(r.Context(), id, optionID); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- brands and suppliers -------------------------------------------------------------

// Brands answers GET /admin/brands.
func (h *Taxonomy) Brands(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.ListBrands(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := make([]dto.Brand, len(list))
	for i, b := range list {
		out[i] = dto.NewBrand(b)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

// CreateBrand answers POST /admin/brands.
func (h *Taxonomy) CreateBrand(w http.ResponseWriter, r *http.Request) {
	var in service.BrandInput
	if !h.decode(w, r, &in) {
		return
	}
	b, err := h.svc.CreateBrand(r.Context(), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, dto.NewBrand(b))
}

// UpdateBrand answers PUT /admin/brands/{id}.
func (h *Taxonomy) UpdateBrand(w http.ResponseWriter, r *http.Request) {
	id, ok := h.idParam(w, r, "id")
	if !ok {
		return
	}
	var in service.BrandInput
	if !h.decode(w, r, &in) {
		return
	}
	b, err := h.svc.UpdateBrand(r.Context(), id, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, dto.NewBrand(b))
}

// DeleteBrand answers DELETE /admin/brands/{id}.
func (h *Taxonomy) DeleteBrand(w http.ResponseWriter, r *http.Request) {
	id, ok := h.idParam(w, r, "id")
	if !ok {
		return
	}
	if err := h.svc.DeleteBrand(r.Context(), id); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Suppliers answers GET /admin/suppliers.
func (h *Taxonomy) Suppliers(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.ListSuppliers(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := make([]dto.Supplier, len(list))
	for i, s := range list {
		out[i] = dto.NewSupplier(s)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

// CreateSupplier answers POST /admin/suppliers.
func (h *Taxonomy) CreateSupplier(w http.ResponseWriter, r *http.Request) {
	var in service.SupplierInput
	if !h.decode(w, r, &in) {
		return
	}
	s, err := h.svc.CreateSupplier(r.Context(), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, dto.NewSupplier(s))
}

// UpdateSupplier answers PUT /admin/suppliers/{id}.
func (h *Taxonomy) UpdateSupplier(w http.ResponseWriter, r *http.Request) {
	id, ok := h.idParam(w, r, "id")
	if !ok {
		return
	}
	var in service.SupplierInput
	if !h.decode(w, r, &in) {
		return
	}
	s, err := h.svc.UpdateSupplier(r.Context(), id, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, dto.NewSupplier(s))
}

// DeleteSupplier answers DELETE /admin/suppliers/{id}.
func (h *Taxonomy) DeleteSupplier(w http.ResponseWriter, r *http.Request) {
	id, ok := h.idParam(w, r, "id")
	if !ok {
		return
	}
	if err := h.svc.DeleteSupplier(r.Context(), id); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
