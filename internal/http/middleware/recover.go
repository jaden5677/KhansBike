package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"
)

// Recoverer converts a panic in any downstream handler into a logged 500 rather
// than a crashed process. This is the one sanctioned place a panic is caught:
// the "no panic outside main" rule is about not *raising* panics as control
// flow; a defensive net at the HTTP boundary keeps one bad handler from taking
// down the single-binary deployment. The full stack is logged, never returned.
func Recoverer(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					logger.LogAttrs(r.Context(), slog.LevelError, "panic_recovered",
						slog.String("request_id", RequestIDFromContext(r.Context())),
						slog.Any("panic", rec),
						slog.String("stack", string(debug.Stack())),
					)
					// TODO(phase-4): emit an RFC 9457 problem+json body via the
					// problem package instead of a bare status once it exists.
					w.Header().Set("Content-Type", "application/problem+json")
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte(`{"type":"about:blank","title":"Internal Server Error","status":500}`))
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
