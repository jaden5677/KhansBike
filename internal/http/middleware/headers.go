package middleware

import "net/http"

// SecureHeaders sets response headers that close common browser attack
// vectors: MIME sniffing, clickjacking via framing, and leaking full URLs in
// the Referer header. In production, HSTS tells browsers to use HTTPS only
// (TLS is terminated at Cloudflare in front of this process).
func SecureHeaders(production bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
			if production {
				h.Set("Strict-Transport-Security", "max-age=31536000")
			}
			next.ServeHTTP(w, r)
		})
	}
}

// CacheControl sets a Cache-Control header on every response of a route
// group: short public caching for the anonymous catalogue (Cloudflare can
// serve repeat reads), no-store for anything authenticated.
func CacheControl(value string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", value)
			next.ServeHTTP(w, r)
		})
	}
}
