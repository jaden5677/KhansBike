// Package domain holds the pure business types and sentinel errors that the rest
// of the application is written in terms of. It depends only on the standard
// library, google/uuid, and the dependency-free platform value types (Money);
// it imports no storage, transport, or third-party framework code, so the core
// model never bends to an infrastructure concern.
package domain

import (
	"errors"
	"fmt"
	"strings"
)

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

// FieldError describes one invalid input field. Field uses the client's
// vocabulary (JSON names, dotted paths like "variants[0].sku") so a form can
// highlight the exact input.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ValidationError collects every problem found in one input, so a client can
// fix them all in one round trip instead of one per request. It matches
// ErrValidation under errors.Is, which keeps a single mapping to 422 in the
// HTTP layer.
type ValidationError struct {
	Fields []FieldError
}

// Add records a problem with one field.
func (e *ValidationError) Add(field, format string, args ...any) {
	e.Fields = append(e.Fields, FieldError{Field: field, Message: fmt.Sprintf(format, args...)})
}

// Err returns e if any problem was recorded, else nil, so callers can collect
// unconditionally and return v.Err() at the end.
func (e *ValidationError) Err() error {
	if len(e.Fields) == 0 {
		return nil
	}
	return e
}

func (e *ValidationError) Error() string {
	msgs := make([]string, len(e.Fields))
	for i, f := range e.Fields {
		msgs[i] = f.Field + ": " + f.Message
	}
	return ErrValidation.Error() + ": " + strings.Join(msgs, "; ")
}

func (e *ValidationError) Unwrap() error { return ErrValidation }

// Invalid is shorthand for a ValidationError with a single field.
func Invalid(field, format string, args ...any) error {
	v := &ValidationError{}
	v.Add(field, format, args...)
	return v
}
