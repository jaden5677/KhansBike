package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func get(h http.Handler, method, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

func TestHandlerServesAppWithRoutingFallback(t *testing.T) {
	h := newHandler(fstest.MapFS{
		"dist/index.html":         {Data: []byte("<html>app</html>")},
		"dist/assets/app-1a2b.js": {Data: []byte("console.log(1)")},
		"dist/robots.txt":         {Data: []byte("User-agent: *")},
		"dist/.env":               {Data: []byte("SECRET=1")},
	}, "dist")

	tests := []struct {
		path, wantBody, wantCache string
	}{
		{"/", "<html>app</html>", "no-cache"},
		{"/products/star-grips", "<html>app</html>", "no-cache"}, // client-side route
		{"/assets/app-1a2b.js", "console.log(1)", "public, max-age=31536000, immutable"},
		{"/robots.txt", "User-agent: *", "no-cache"},
		{"/.env", "<html>app</html>", "no-cache"}, // dotfiles are never served
	}
	for _, tc := range tests {
		rec := get(h, http.MethodGet, tc.path)
		if rec.Code != http.StatusOK || rec.Body.String() != tc.wantBody || rec.Header().Get("Cache-Control") != tc.wantCache {
			t.Errorf("GET %s = %d %q (cache %q)", tc.path, rec.Code, rec.Body.String(), rec.Header().Get("Cache-Control"))
		}
	}
	if rec := get(h, http.MethodPost, "/"); rec.Code != http.StatusNotFound {
		t.Errorf("POST / = %d, want 404", rec.Code)
	}
}

func TestHandlerWithoutBuild(t *testing.T) {
	// What a fresh checkout embeds: only the placeholder that keeps the
	// folder in git. (Not Handler(): a developer's local build would pass.)
	h := newHandler(fstest.MapFS{"dist/.gitkeep": {}}, "dist")
	for _, p := range []string{"/", "/products/star-grips", "/.gitkeep"} {
		if rec := get(h, http.MethodGet, p); rec.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", p, rec.Code)
		}
	}
}
