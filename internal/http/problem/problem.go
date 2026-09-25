// Package problem renders errors as RFC 9457 "problem details" JSON and maps
// the domain's sentinel errors to HTTP status codes. It is the single place
// that decides which status an error becomes, so handlers never pick status
// codes for failures themselves.
package problem

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/khansbikezone/bikezone-api/internal/domain"
)

// Problem is an RFC 9457 problem details body. Errors is an extension member
// listing per-field validation failures.
type Problem struct {
	Type   string              `json:"type"`
	Title  string              `json:"title"`
	Status int                 `json:"status"`
	Detail string              `json:"detail,omitempty"`
	Errors []domain.FieldError `json:"errors,omitempty"`
}

// New builds a problem with the standard title for status. RFC 9457 allows
// "about:blank" as the type when the status code says everything needed.
func New(status int, detail string) Problem {
	return Problem{Type: "about:blank", Title: http.StatusText(status), Status: status, Detail: detail}
}

// FromError maps an error returned by the service layer to a problem. Client
// errors carry the error text as detail: those messages are written by this
// codebase (or curated by the store) and are safe to show. Anything
// unrecognised is a 500 with a generic detail; the caller logs the real error.
func FromError(err error) Problem {
	var verr *domain.ValidationError
	if errors.As(err, &verr) {
		p := New(http.StatusUnprocessableEntity, "The request contains invalid fields.")
		p.Errors = verr.Fields
		return p
	}
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return New(http.StatusRequestEntityTooLarge, "The request body is too large.")
	}
	switch {
	case errors.Is(err, domain.ErrValidation):
		return New(http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, domain.ErrNotFound):
		return New(http.StatusNotFound, err.Error())
	case errors.Is(err, domain.ErrConflict):
		return New(http.StatusConflict, err.Error())
	case errors.Is(err, domain.ErrVersionMismatch):
		return New(http.StatusPreconditionFailed, "The resource was changed by someone else; reload it and try again.")
	case errors.Is(err, domain.ErrUnauthorized):
		return New(http.StatusUnauthorized, err.Error())
	case errors.Is(err, domain.ErrForbidden):
		return New(http.StatusForbidden, err.Error())
	default:
		return New(http.StatusInternalServerError, "An unexpected error occurred.")
	}
}

// Write sends p with the application/problem+json media type. Problems are
// never cacheable: a cached error would outlive its cause.
func Write(w http.ResponseWriter, p Problem) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(p.Status)
	_ = json.NewEncoder(w).Encode(p)
}
