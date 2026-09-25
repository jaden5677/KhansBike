package middleware

import (
	"net/http"
	"net/netip"
	"strconv"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/khansbikezone/bikezone-api/internal/http/problem"
)

// RateLimit allows each client address perMinute requests per minute, with
// bursts of up to burst, and answers 429 beyond that. It protects the
// endpoints that can be abused anonymously: login (password guessing),
// device pairing (code guessing) and mailing-list signup (inbox flooding).
//
// State is in memory: the API is a single process, so there is no shared
// store to coordinate with. Idle entries are swept lazily as requests arrive,
// so the map cannot grow without bound and no background goroutine is needed.
func RateLimit(perMinute, burst int) func(http.Handler) http.Handler {
	l := &limiter{
		every:   rate.Every(time.Minute / time.Duration(perMinute)),
		burst:   burst,
		clients: map[netip.Addr]*client{},
	}
	retryAfter := strconv.Itoa(max(1, 60/perMinute))
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !l.allow(ClientIPFrom(r.Context()), time.Now()) {
				w.Header().Set("Retry-After", retryAfter)
				problem.Write(w, problem.New(http.StatusTooManyRequests, "Too many requests; slow down and try again shortly."))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

type client struct {
	lim      *rate.Limiter
	lastSeen time.Time
}

type limiter struct {
	mu        sync.Mutex
	every     rate.Limit
	burst     int
	clients   map[netip.Addr]*client
	lastSweep time.Time
}

const idleTTL = 10 * time.Minute

func (l *limiter) allow(ip netip.Addr, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if now.Sub(l.lastSweep) > time.Minute {
		for k, c := range l.clients {
			if now.Sub(c.lastSeen) > idleTTL {
				delete(l.clients, k)
			}
		}
		l.lastSweep = now
	}
	c, ok := l.clients[ip]
	if !ok {
		c = &client{lim: rate.NewLimiter(l.every, l.burst)}
		l.clients[ip] = c
	}
	c.lastSeen = now
	return c.lim.AllowN(now, 1)
}
