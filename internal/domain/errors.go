// Package domain holds the pure business types and sentinel errors that the rest
// of the application is written in terms of. It depends only on the standard
// library, google/uuid, and the dependency-free platform value types (Money);
// it imports no storage, transport, or third-party framework code, so the core
// model never bends to an infrastructure concern.
package domain

import "errors"

// The sentinel errors below are the vocabulary the service layer returns and the
// HTTP layer maps to problem responses in exactly one place. Wrapping with %w
// preserves the sentinel so handlers can errors.Is against it while still adding
// context (which entity, which id).
var (
	// ErrNotFound means a requested entity does not exist (or is not visible to
	// the caller, which the public path deliberately conflates with absence).
	ErrNotFound = errors.New("not found")

	// ErrConflict means the write collided with existing state, e.g. a duplicate
	// slug or SKU.
	ErrConflict = errors.New("conflict")

	// ErrValidation means the input failed a business rule that the type system
	// could not express (e.g. wrong value column for an attribute's data type).
	ErrValidation = errors.New("validation failed")

	// ErrVersionMismatch backs optimistic concurrency: the client's If-Match
	// ETag did not match the current row version. Handlers map this to 412.
	ErrVersionMismatch = errors.New("version mismatch")

	// ErrUnauthorized means no valid credential was presented.
	ErrUnauthorized = errors.New("unauthorized")

	// ErrForbidden means a valid credential lacks the required role.
	ErrForbidden = errors.New("forbidden")
)
