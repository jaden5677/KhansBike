package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/khansbikezone/bikezone-api/internal/domain"
	"github.com/khansbikezone/bikezone-api/internal/store/gen"
)

// Audit appends an audit entry attributed to the actor in ctx. before and
// after are JSON snapshots of the entity around the change; pass nil for
// "nothing" (a create has no before, a delete has no after). Call it inside
// the transaction that makes the change, so the log and the data agree.
func (q *Queries) Audit(ctx context.Context, action, entityType string, entityID *uuid.UUID, before, after any) error {
	b, err := snapshot(before)
	if err != nil {
		return err
	}
	a, err := snapshot(after)
	if err != nil {
		return err
	}
	actor := domain.ActorFrom(ctx)
	params := gen.InsertAuditLogParams{
		ID:          domain.NewID(),
		ActorUserID: actor.UserID,
		ActorKind:   string(actor.Kind),
		Action:      action,
		EntityType:  entityType,
		EntityID:    entityID,
		Before:      b,
		After:       a,
	}
	if actor.IP.IsValid() {
		ip := actor.IP
		params.Ip = &ip
	}
	if err := q.InsertAuditLog(ctx, params); err != nil {
		return fmt.Errorf("write audit entry: %w", err)
	}
	return nil
}

func snapshot(v any) ([]byte, error) {
	if v == nil {
		return nil, nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("audit snapshot: %w", err)
	}
	if string(b) == "null" { // a typed nil pointer
		return nil, nil
	}
	return b, nil
}

// defaultMaxAttempts bounds retries before a job is dead-lettered.
const defaultMaxAttempts = 5

// Enqueue adds a background job. Call it in the same transaction as the
// change that needs the job (a transactional outbox): the job then exists if
// and only if the change committed.
func (q *Queries) Enqueue(ctx context.Context, kind string, payload any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode %s job payload: %w", kind, err)
	}
	if _, err := q.EnqueueJob(ctx, gen.EnqueueJobParams{
		ID:          domain.NewID(),
		Kind:        kind,
		Payload:     b,
		MaxAttempts: defaultMaxAttempts,
	}); err != nil {
		return fmt.Errorf("enqueue %s job: %w", kind, err)
	}
	return nil
}
