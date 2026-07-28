package domain

import (
	"time"

	"github.com/google/uuid"
)

// Role enumerates the authorisation levels. Only RoleAdmin is issued in v1;
// RoleWholesale exists so a future trade-login feature is a data change, not a
// schema/type migration, matching the reserved value in the users table.
type Role string

const (
	// RoleAdmin has full CRUD over the catalogue, media, attributes, and
	// categories, and sees every price tier.
	RoleAdmin Role = "admin"
	// RoleWholesale is reserved for v2 trade customers; unused in v1.
	RoleWholesale Role = "wholesale"
)

// ActorKind distinguishes how an authenticated request proved its identity,
// which the audit log records so an action can be traced to the desktop session
// or the paired phone.
type ActorKind string

const (
	// ActorAdmin is a browser session authenticated by cookie.
	ActorAdmin ActorKind = "admin"
	// ActorDevice is the owner's phone authenticated by a device token.
	ActorDevice ActorKind = "device"
	// ActorSystem is the server itself (jobs, importer) acting without a user.
	ActorSystem ActorKind = "system"
)

// User is an authenticated principal. Only admins exist in v1. PasswordHash is
// an argon2id encoded string and never leaves the server.
type User struct {
	ID               uuid.UUID
	Email            string
	PasswordHash     string
	Role             Role
	DisplayName      string
	FailedLoginCount int
	LockedUntil      *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// IsLocked reports whether the account is currently within a lockout window,
// used by the auth service to reject login attempts without touching the hash.
func (u *User) IsLocked(now time.Time) bool {
	return u.LockedUntil != nil && u.LockedUntil.After(now)
}

// Session is a browser session backed by the database. The raw token is only
// ever held by the client cookie; the server stores TokenHash (a SHA-256) so a
// database leak does not yield usable session tokens.
type Session struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	TokenHash  []byte
	UserAgent  *string
	IP         *string
	ExpiresAt  time.Time
	CreatedAt  time.Time
	LastSeenAt time.Time
}

// DeviceToken authenticates the owner's phone. Like sessions, only the hash is
// stored. RevokedAt being non-nil disables the device without deleting its
// history.
type DeviceToken struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	Name       string
	TokenHash  []byte
	LastSeenAt *time.Time
	RevokedAt  *time.Time
	CreatedAt  time.Time
}

// PairingCode is the short-lived one-time code shown as a QR to bootstrap a
// device token, so the phone never types a password.
type PairingCode struct {
	Code       string
	UserID     uuid.UUID
	ExpiresAt  time.Time
	ConsumedAt *time.Time
}
