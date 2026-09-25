package auth

import (
	"context"

	"github.com/google/uuid"

	"github.com/khansbikezone/bikezone-api/internal/domain"
)

// Principal is an authenticated caller: a user plus the credential they
// presented. Exactly one of SessionID (browser, cookie) or DeviceID (paired
// phone, bearer token) is set, per Kind.
type Principal struct {
	UserID      uuid.UUID
	Email       string
	DisplayName string
	Role        domain.Role
	Kind        domain.ActorKind // ActorAdmin for sessions, ActorDevice for devices
	SessionID   uuid.UUID
	DeviceID    uuid.UUID
}

// IsSession reports whether the caller authenticated with a browser session
// (and therefore must present a CSRF token on writes).
func (p *Principal) IsSession() bool { return p.Kind == domain.ActorAdmin }

type principalKey struct{}

// WithPrincipal returns a context carrying p.
func WithPrincipal(ctx context.Context, p *Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

// PrincipalFrom returns the authenticated caller, or nil if the request is
// anonymous.
func PrincipalFrom(ctx context.Context) *Principal {
	p, _ := ctx.Value(principalKey{}).(*Principal)
	return p
}
