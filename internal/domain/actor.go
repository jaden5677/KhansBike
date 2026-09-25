package domain

import (
	"context"
	"net/netip"
	"time"

	"github.com/google/uuid"
)

// Actor is whoever is performing the current operation: the admin in a
// browser session, the owner's paired phone, or the server itself. Services
// read it from the context to attribute audit entries; it is attached once by
// the authentication middleware (or by a job runner / CLI for system work).
type Actor struct {
	UserID *uuid.UUID // nil for ActorSystem
	Kind   ActorKind
	IP     netip.Addr // zero (invalid) when not request-driven
}

// SystemActor is the actor for work the server performs on its own behalf.
var SystemActor = Actor{Kind: ActorSystem}

type actorKey struct{}

// WithActor returns a context carrying a.
func WithActor(ctx context.Context, a Actor) context.Context {
	return context.WithValue(ctx, actorKey{}, a)
}

// ActorFrom returns the actor attached to ctx, defaulting to SystemActor so a
// missing attachment is attributed conservatively rather than to a user.
func ActorFrom(ctx context.Context) Actor {
	if a, ok := ctx.Value(actorKey{}).(Actor); ok {
		return a
	}
	return SystemActor
}

// AuditEntry is one row of the append-only audit log. Before and After hold
// JSON snapshots of the entity around the change (nil for creates/deletes).
type AuditEntry struct {
	ID          uuid.UUID
	ActorUserID *uuid.UUID
	ActorEmail  *string
	ActorKind   ActorKind
	Action      string // e.g. "product.update"
	EntityType  string // e.g. "product"
	EntityID    *uuid.UUID
	Before      []byte
	After       []byte
	IP          *netip.Addr
	CreatedAt   time.Time
}
