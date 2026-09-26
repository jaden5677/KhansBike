package middleware

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"

	"github.com/khansbikezone/bikezone-api/internal/http/problem"
)

func TestClientIPTrustsHeaderOnlyFromProxies(t *testing.T) {
	var got netip.Addr
	h := ClientIP(true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = ClientIPFrom(r.Context())
	}))
	tests := []struct {
		name, remote, header, want string
	}{
		{"tunnel_on_loopback", "127.0.0.1:50000", "203.0.113.9", "203.0.113.9"},
		{"cloudflare_edge", "104.16.1.1:443", "203.0.113.9", "203.0.113.9"},
		{"spoofed_from_lan", "192.168.1.20:51000", "203.0.113.9", "192.168.1.20"},
		{"garbage_header", "127.0.0.1:50000", "not-an-ip", "127.0.0.1"},
		{"ipv6_loopback", "[::1]:50000", "2001:db8::1", "2001:db8::1"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = tc.remote
			r.Header.Set("CF-Connecting-IP", tc.header)
			h.ServeHTTP(httptest.NewRecorder(), r)
			if got.String() != tc.want {
				t.Errorf("client ip = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestClientIPIgnoresHeaderWhenNotTrusted(t *testing.T) {
	var got netip.Addr
	h := ClientIP(false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { got = ClientIPFrom(r.Context()) }))
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "127.0.0.1:1"
	r.Header.Set("CF-Connecting-IP", "203.0.113.9")
	h.ServeHTTP(httptest.NewRecorder(), r)
	if got.String() != "127.0.0.1" {
		t.Errorf("client ip = %s, want the peer address", got)
	}
}

func TestLimiter(t *testing.T) {
	l := &limiter{every: 1, burst: 2, clients: map[netip.Addr]*client{}} // 1/s, burst 2
	a, b := netip.MustParseAddr("10.0.0.1"), netip.MustParseAddr("10.0.0.2")
	now := time.Now()
	for i := 0; i < 2; i++ {
		if !l.allow(a, now) {
			t.Fatalf("request %d within the burst was refused", i+1)
		}
	}
	if l.allow(a, now) {
		t.Error("third immediate request allowed past the burst")
	}
	if !l.allow(b, now) {
		t.Error("a different client was limited")
	}
	if !l.allow(a, now.Add(1100*time.Millisecond)) {
		t.Error("request not allowed after the refill interval")
	}
	l.allow(b, now.Add(time.Hour)) // triggers a sweep of idle entries
	if _, ok := l.clients[a]; ok {
		t.Error("idle client not swept")
	}
}

func TestRateLimitAnswers429(t *testing.T) {
	h := ClientIP(false)(RateLimit(1, 1)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})))
	send := func() *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/", nil)
		r.RemoteAddr = "10.1.1.1:1"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		return rec
	}
	if send().Code != http.StatusOK {
		t.Fatal("first request limited")
	}
	rec := send()
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
		t.Errorf("second request: %d, Retry-After %q", rec.Code, rec.Header().Get("Retry-After"))
	}
}

func TestRequestIDRejectsUnsafeInbound(t *testing.T) {
	var got string
	h := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { got = RequestIDFromContext(r.Context()) }))
	for inbound, keep := range map[string]bool{
		"0192c1f2-8f5e-7abc-9def-0123456789ab": true,
		"8a1b2c3d4e5f6a7b-IAD":                 true,
		"evil\nid":                             false,
		string(make([]byte, 500)):              false,
	} {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("X-Request-ID", inbound)
		h.ServeHTTP(httptest.NewRecorder(), r)
		if (got == inbound) != keep {
			t.Errorf("inbound %q: kept=%v, want %v", inbound, got == inbound, keep)
		}
	}
}

func TestRecovererWritesProblemAndRepanicsAbort(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := Recoverer(log)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { panic("boom") }))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusInternalServerError || rec.Header().Get("Content-Type") != "application/problem+json" {
		t.Errorf("panic produced %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}

	abort := Recoverer(log)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { panic(http.ErrAbortHandler) }))
	defer func() {
		if rec := recover(); rec == nil || !errors.Is(rec.(error), http.ErrAbortHandler) {
			t.Errorf("ErrAbortHandler was swallowed: %v", rec)
		}
	}()
	abort.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
}

func TestAuthenticateQuietlyDropsAbandonedRequests(t *testing.T) {
	var logs bytes.Buffer
	log := slog.New(slog.NewTextHandler(&logs, nil))
	// No cookie or token, so the auth service is never reached.
	h := Authenticate(nil, "bz_session", log)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("the handler ran for an unauthenticated request")
	}))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/v1/admin/products", nil))
	if rec.Code != problem.StatusClientClosedRequest || logs.Len() != 0 {
		t.Errorf("cancelled request: status %d, logs %q; want 499 and nothing logged", rec.Code, logs.String())
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/admin/products", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("no credentials: status %d, want 401", rec.Code)
	}
}
