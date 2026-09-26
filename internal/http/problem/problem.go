// Package problem renders errors as RFC 9457 "problem details" JSON and maps
// the domain's sentinel errors to HTTP status codes. It is the single place
// that decides which status an error becomes, so handlers never pick status
// codes for failures themselves.
package problem

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"

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
		return New(http.StatusUnprocessableEntity, sentence(err, domain.ErrValidation))
	case errors.Is(err, domain.ErrNotFound):
		return New(http.StatusNotFound, sentence(err, domain.ErrNotFound))
	case errors.Is(err, domain.ErrConflict):
		return New(http.StatusConflict, sentence(err, domain.ErrConflict))
	case errors.Is(err, domain.ErrVersionMismatch):
		return New(http.StatusPreconditionFailed, "The resource was changed by someone else; reload it and try again.")
	case errors.Is(err, domain.ErrUnauthorized):
		return New(http.StatusUnauthorized, sentence(err, domain.ErrUnauthorized))
	case errors.Is(err, domain.ErrForbidden):
		return New(http.StatusForbidden, sentence(err, domain.ErrForbidden))
	default:
		return New(http.StatusInternalServerError, "An unexpected error occurred.")
	}
}

// sentence turns a wrapped domain error into the message a person reads.
// Errors are built as fmt.Errorf("%w: the image is already attached",
// domain.ErrConflict); the kind is already in the status code, so the
// "conflict: " prefix is dropped and the rest starts with a capital:
// "The image is already attached".
func sentence(err, kind error) string {
	msg := strings.TrimPrefix(err.Error(), kind.Error()+": ")
	r, size := utf8.DecodeRuneInString(msg)
	return string(unicode.ToUpper(r)) + msg[size:]
}

// Write sends p with the application/problem+json media type. Problems are
// never cacheable: a cached error would outlive its cause.
func Write(w http.ResponseWriter, p Problem) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(p.Status)
	_ = json.NewEncoder(w).Encode(p)
}

// StatusClientClosedRequest is nginx's non-standard status for a request the
// client abandoned (closed the tab, or the web app cancelled it). Nobody
// receives the response, so it only ever appears in our access logs, where
// it keeps such requests apart from real server errors.
const StatusClientClosedRequest = 499
