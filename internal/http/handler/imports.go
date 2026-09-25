package handler

import (
	"log/slog"
	"net/http"

	"github.com/khansbikezone/bikezone-api/internal/domain"
	"github.com/khansbikezone/bikezone-api/internal/http/dto"
	"github.com/khansbikezone/bikezone-api/internal/importer"
)

// Imports serves the workbook import workflow: stage, review, commit.
type Imports struct {
	base
	svc *importer.Importer
}

// NewImports builds the import handlers.
func NewImports(log *slog.Logger, svc *importer.Importer) *Imports {
	return &Imports{base: base{log: log}, svc: svc}
}

// Stage answers POST /admin/imports: upload a workbook (multipart part
// "file") and get back a dry-run batch to review. Nothing in the catalogue
// changes until the batch is committed.
func (h *Imports) Stage(w http.ResponseWriter, r *http.Request) {
	part, ok := h.filePart(w, r, importer.MaxWorkbookBytes)
	if !ok {
		return
	}
	defer func() { _ = part.Close() }()
	b, err := h.svc.Stage(r.Context(), part.FileName(), part)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, dto.NewImportBatch(b))
}

// Batches answers GET /admin/imports.
func (h *Imports) Batches(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.Batches(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := make([]dto.ImportBatch, len(list))
	for i, b := range list {
		out[i] = dto.NewImportBatch(b)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

// Batch answers GET /admin/imports/{id}.
func (h *Imports) Batch(w http.ResponseWriter, r *http.Request) {
	id, ok := h.idParam(w, r, "id")
	if !ok {
		return
	}
	b, err := h.svc.Batch(r.Context(), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, dto.NewImportBatch(b))
}

// Rows answers GET /admin/imports/{id}/rows?decision=&issues=true&cursor=&limit=.
func (h *Imports) Rows(w http.ResponseWriter, r *http.Request) {
	id, ok := h.idParam(w, r, "id")
	if !ok {
		return
	}
	v := r.URL.Query()
	limit, err := queryInt(v, "limit")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	rows, next, err := h.svc.Rows(r.Context(), id, importer.RowQuery{
		Decision:       domain.ImportDecision(v.Get("decision")),
		WithIssuesOnly: v.Get("issues") == "true",
		Cursor:         v.Get("cursor"),
		Limit:          limit,
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	page := dto.Page[dto.ImportRow]{Items: make([]dto.ImportRow, len(rows)), NextCursor: next}
	for i, row := range rows {
		page.Items[i] = dto.NewImportRow(row)
	}
	writeJSON(w, http.StatusOK, page)
}

// Decide answers PUT /admin/imports/{id}/rows/{rowId} with {"decision": ...}.
func (h *Imports) Decide(w http.ResponseWriter, r *http.Request) {
	id, ok := h.idParam(w, r, "id")
	if !ok {
		return
	}
	rowID, ok := h.idParam(w, r, "rowId")
	if !ok {
		return
	}
	var in struct {
		Decision domain.ImportDecision `json:"decision"`
	}
	if !h.decode(w, r, &in) {
		return
	}
	row, err := h.svc.SetDecision(r.Context(), id, rowID, in.Decision)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, dto.NewImportRow(row))
}

// Commit answers POST /admin/imports/{id}/commit.
func (h *Imports) Commit(w http.ResponseWriter, r *http.Request) {
	id, ok := h.idParam(w, r, "id")
	if !ok {
		return
	}
	var opts importer.CommitOptions
	if r.ContentLength != 0 && !h.decode(w, r, &opts) {
		return
	}
	b, err := h.svc.Commit(r.Context(), id, opts)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, dto.NewImportBatch(b))
}

// Abort answers POST /admin/imports/{id}/abort.
func (h *Imports) Abort(w http.ResponseWriter, r *http.Request) {
	id, ok := h.idParam(w, r, "id")
	if !ok {
		return
	}
	if err := h.svc.Abort(r.Context(), id); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
