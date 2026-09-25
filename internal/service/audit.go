package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/khansbikezone/bikezone-api/internal/domain"
	"github.com/khansbikezone/bikezone-api/internal/platform"
	"github.com/khansbikezone/bikezone-api/internal/store"
	"github.com/khansbikezone/bikezone-api/internal/store/gen"
)

// Audit reads the append-only audit log.
type Audit struct {
	store *store.Store
}

// NewAudit builds the audit log reader.
func NewAudit(st *store.Store) *Audit { return &Audit{store: st} }

// AuditQuery narrows the log. EntityType and EntityID are optional.
type AuditQuery struct {
	EntityType string
	EntityID   *uuid.UUID
	Cursor     string
	Limit      int
}

type auditCursor struct {
	At time.Time `json:"t"`
	ID uuid.UUID `json:"i"`
}

// List returns log entries newest first, one keyset page at a time.
func (a *Audit) List(ctx context.Context, aq AuditQuery) (entries []domain.AuditEntry, next string, err error) {
	limit := platform.ClampLimit(aq.Limit, 50, 200)
	// The first page starts after "the end of time" and the largest id.
	c := auditCursor{At: time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC), ID: uuid.Max}
	if aq.Cursor != "" {
		if err := platform.DecodeCursor(aq.Cursor, &c); err != nil {
			return nil, "", domain.Invalid("cursor", "is invalid")
		}
	}
	rows, err := a.store.ListAuditLog(ctx, gen.ListAuditLogParams{
		EntityType:      store.TextOrNull(strings.TrimSpace(aq.EntityType)),
		EntityID:        aq.EntityID,
		BeforeCreatedAt: store.Timestamptz(c.At),
		BeforeID:        c.ID,
		RowLimit:        int32(limit + 1),
	})
	if err != nil {
		return nil, "", fmt.Errorf("list audit log: %w", err)
	}
	for _, r := range rows {
		entries = append(entries, store.AuditEntry(r))
	}
	if len(entries) > limit {
		entries = entries[:limit]
		last := entries[limit-1]
		if next, err = platform.EncodeCursor(auditCursor{At: last.CreatedAt, ID: last.ID}); err != nil {
			return nil, "", err
		}
	}
	return entries, next, nil
}
