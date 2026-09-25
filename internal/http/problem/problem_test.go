package problem

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/khansbikezone/bikezone-api/internal/domain"
)

func TestFromErrorStatusMapping(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"not_found", fmt.Errorf("product %q: %w", "x", domain.ErrNotFound), http.StatusNotFound},
		{"conflict", fmt.Errorf("%w: slug taken", domain.ErrConflict), http.StatusConflict},
		{"validation_sentinel", fmt.Errorf("%w: bad", domain.ErrValidation), http.StatusUnprocessableEntity},
		{"validation_fields", domain.Invalid("name", "is required"), http.StatusUnprocessableEntity},
		{"version", domain.ErrVersionMismatch, http.StatusPreconditionFailed},
		{"unauthorized", domain.ErrUnauthorized, http.StatusUnauthorized},
		{"forbidden", domain.ErrForbidden, http.StatusForbidden},
		{"too_large", &http.MaxBytesError{Limit: 10}, http.StatusRequestEntityTooLarge},
		{"unknown", errors.New("connection reset by peer"), http.StatusInternalServerError},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := FromError(tc.err).Status; got != tc.want {
				t.Errorf("FromError(%v).Status = %d, want %d", tc.err, got, tc.want)
			}
		})
	}
}

func TestFromErrorHidesInternalDetail(t *testing.T) {
	p := FromError(errors.New(`pq: relation "secret_table" does not exist`))
	if p.Detail != "An unexpected error occurred." {
		t.Errorf("500 detail leaked internals: %q", p.Detail)
	}
}

func TestWriteValidationBody(t *testing.T) {
	rec := httptest.NewRecorder()
	Write(rec, FromError(domain.Invalid("variants[0].sku", "is required")))

	if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Errorf("Content-Type = %q", ct)
	}
	var body Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Status != http.StatusUnprocessableEntity || len(body.Errors) != 1 || body.Errors[0].Field != "variants[0].sku" {
		t.Errorf("unexpected body: %+v", body)
	}
}
