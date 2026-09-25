package middleware

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// cloudflareRanges are Cloudflare's published edge ranges
// (https://www.cloudflare.com/ips/). They change rarely; review them when
// upgrading. Behind a Cloudflare Tunnel the TCP peer is cloudflared on the
// same machine (loopback), which is trusted too.
var cloudflareRanges = mustPrefixes(
	"173.245.48.0/20", "103.21.244.0/22", "103.22.200.0/22", "103.31.4.0/22",
	"141.101.64.0/18", "108.162.192.0/18", "190.93.240.0/20", "188.114.96.0/20",
	"197.234.240.0/22", "198.41.128.0/17", "162.158.0.0/15", "104.16.0.0/13",
	"104.24.0.0/14", "172.64.0.0/13", "131.0.72.0/22",
	"2400:cb00::/32", "2606:4700::/32", "2803:f800::/32", "2405:b500::/32",
	"2405:8100::/32", "2a06:98c0::/29", "2c0f:f248::/32",
)

func mustPrefixes(cidrs ...string) []netip.Prefix {
	out := make([]netip.Prefix, len(cidrs))
	for i, c := range cidrs {
		out[i] = netip.MustParsePrefix(c) // constants: a typo fails at startup, in tests
	}
	return out
}

func trustedProxy(peer netip.Addr) bool {
	if peer.IsLoopback() {
		return true
	}
	for _, p := range cloudflareRanges {
		if p.Contains(peer) {
			return true
		}
	}
	return false
}

type clientIPKey struct{}

// ClientIP determines the caller's address for rate limiting, audit and logs.
// With trustCloudflare it honours the CF-Connecting-IP header, but only when
// the request actually arrived from a trusted proxy (loopback, where
// cloudflared runs, or a Cloudflare edge address). Otherwise anyone who can
// reach the port could claim any address by sending the header themselves.
func ClientIP(trustCloudflare bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := peerAddr(r.RemoteAddr)
			if trustCloudflare && ip.IsValid() && trustedProxy(ip) {
				if h, err := netip.ParseAddr(strings.TrimSpace(r.Header.Get("CF-Connecting-IP"))); err == nil {
					ip = h
				}
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), clientIPKey{}, ip.Unmap())))
		})
	}
}

func peerAddr(remote string) netip.Addr {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}
	}
	return ip
}

// ClientIPFrom returns the address resolved by ClientIP (invalid if the
// middleware did not run).
func ClientIPFrom(ctx context.Context) netip.Addr {
	ip, _ := ctx.Value(clientIPKey{}).(netip.Addr)
	return ip
}
