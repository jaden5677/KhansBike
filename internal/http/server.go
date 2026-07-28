package http

import (
	"net/http"
	"time"

	"github.com/khansbikezone/bikezone-api/internal/config"
)

// NewServer constructs the http.Server with hardened timeouts. TLS is
// deliberately absent: a Cloudflare Tunnel terminates TLS in front of this
// process, which only ever listens on loopback. The timeouts below bound how
// long a slow or malicious client can tie up a connection — important for a box
// sitting in a shop back office with finite resources.
func NewServer(cfg *config.Config, h http.Handler) *http.Server {
	return &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20, // 1 MiB
	}
}
