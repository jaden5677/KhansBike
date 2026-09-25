package middleware

import (
	"errors"
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/khansbikezone/bikezone-api/internal/http/problem"
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
					handlePanic(logger, w, r, rec)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// handlePanic logs a recovered panic with its stack and answers 500.
func handlePanic(logger *slog.Logger, w http.ResponseWriter, r *http.Request, rec any) {
	// http.ErrAbortHandler is the sanctioned way to abort a response; the
	// server handles it quietly, so let it through.
	if err, ok := rec.(error); ok && errors.Is(err, http.ErrAbortHandler) {
		panic(rec)
	}
	logger.LogAttrs(r.Context(), slog.LevelError, "panic_recovered",
		slog.String("request_id", RequestIDFromContext(r.Context())),
		slog.Any("panic", rec),
		slog.String("stack", string(debug.Stack())),
	)
	// If the handler had already started its response, appending a problem
	// body would corrupt it; the truncated response will have to do.
	if sr, ok := w.(*statusRecorder); ok && sr.wroteHeader {
		return
	}
	problem.Write(w, problem.New(http.StatusInternalServerError, "An unexpected error occurred."))
}
