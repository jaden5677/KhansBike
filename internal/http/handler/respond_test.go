package handler

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/khansbikezone/bikezone-api/internal/http/problem"
)

func TestFailQuietlyDropsCancelledRequests(t *testing.T) {
	var logs bytes.Buffer
	b := base{log: slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelInfo}))}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // the browser gave up on the request
	r := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/v1/products", nil)
	w := httptest.NewRecorder()
	b.fail(w, r, context.Canceled)

	if w.Code != problem.StatusClientClosedRequest {
		t.Errorf("status = %d, want %d", w.Code, problem.StatusClientClosedRequest)
	}
	if logs.Len() != 0 {
		t.Errorf("a cancelled request was logged at info or above: %s", logs.String())
	}
}

func TestFailLogsServerErrors(t *testing.T) {
	var logs bytes.Buffer
	b := base{log: slog.New(slog.NewTextHandler(&logs, nil))}

	r := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/products", nil)
	w := httptest.NewRecorder()
	b.fail(w, r, errors.New("connection refused"))

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
	if !strings.Contains(logs.String(), "level=ERROR") {
		t.Errorf("server error was not logged: %q", logs.String())
	}
	if strings.Contains(w.Body.String(), "connection refused") {
		t.Error("internal error text leaked to the client")
	}
}
