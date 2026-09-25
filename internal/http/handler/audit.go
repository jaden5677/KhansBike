package handler

import (
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"github.com/khansbikezone/bikezone-api/internal/domain"
	"github.com/khansbikezone/bikezone-api/internal/http/dto"
	"github.com/khansbikezone/bikezone-api/internal/service"
)

// Audit serves the audit log.
type Audit struct {
	base
	svc *service.Audit
}

// NewAudit builds the audit log handler.
func NewAudit(log *slog.Logger, svc *service.Audit) *Audit {
	return &Audit{base: base{log: log}, svc: svc}
}

// List answers GET /admin/audit?entityType=&entityId=&cursor=&limit=,
// newest first.
func (h *Audit) List(w http.ResponseWriter, r *http.Request) {
	v := r.URL.Query()
	limit, err := queryInt(v, "limit")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	q := service.AuditQuery{EntityType: v.Get("entityType"), Cursor: v.Get("cursor"), Limit: limit}
	if s := v.Get("entityId"); s != "" {
		id, err := uuid.Parse(s)
		if err != nil {
			h.fail(w, r, domain.Invalid("entityId", "must be a UUID"))
			return
		}
		q.EntityID = &id
	}
	entries, next, err := h.svc.List(r.Context(), q)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	page := dto.Page[dto.AuditEntry]{Items: make([]dto.AuditEntry, len(entries)), NextCursor: next}
	for i, e := range entries {
		page.Items[i] = dto.NewAuditEntry(e)
	}
	writeJSON(w, http.StatusOK, page)
}
