// Package middleware holds the cross-cutting HTTP middleware that every route
// shares: request identity, structured access logging, and panic recovery. Each
// concern lives in its own file so the router reads as a short, declarative list.
package middleware

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

// requestIDHeader is the canonical header used to carry a request's correlation
// id both inbound (from Cloudflare / a client) and outbound (echoed to the caller).
const requestIDHeader = "X-Request-ID"

type ctxKey int

const requestIDKey ctxKey = iota

// maxRequestIDLen bounds an inbound id: it is echoed into every log line.
const maxRequestIDLen = 128

// validRequestID accepts the characters real tracing ids use (uuids, hex,
// ULIDs, Cloudflare ray ids). Anything else could forge or garble log output.
func validRequestID(id string) bool {
	if id == "" || len(id) > maxRequestIDLen {
		return false
	}
	for i := 0; i < len(id); i++ {
		if !isIDChar(id[i]) {
			return false
		}
	}
	return true
}

func isIDChar(c byte) bool {
	switch {
	case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		return true
	default:
		return c == '-' || c == '_' || c == '.' || c == ':'
	}
}

// RequestID ensures every request has a stable correlation id. It reuses a
// well-formed inbound X-Request-ID (so a trace survives the Cloudflare hop) and
// otherwise mints a UUIDv7, which sorts by creation time and groups naturally in
// logs. The id is stored in the context and echoed in the response header.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(requestIDHeader)
		if !validRequestID(id) {
			if v, err := uuid.NewV7(); err == nil {
				id = v.String()
			} else {
				id = uuid.NewString()
			}
		}
		w.Header().Set(requestIDHeader, id)
		ctx := context.WithValue(r.Context(), requestIDKey, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequestIDFromContext returns the correlation id attached by RequestID, or ""
// if the middleware did not run. Callers use it to tag logs and problem
// responses with the same id the client sees.
func RequestIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey).(string); ok {
		return id
	}
	return ""
}
