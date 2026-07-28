// Package handler contains the HTTP handlers that make up the API surface. Each
// file groups a cohesive set of endpoints; this one holds the liveness and
// readiness probes used by the Windows service manager and the tunnel.
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// Pinger is the minimal dependency the readiness probe needs: something that can
// confirm the datastore is reachable. It is an interface (not the concrete pool)
// so health checks stay decoupled from the storage layer and are trivial to fake
// in tests. A nil Pinger means "readiness does not depend on a database yet",
// which is the state during early-checkpoint boots.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Health wires the liveness/readiness endpoints to their dependencies.
type Health struct {
	db Pinger
}

// NewHealth constructs the health handler. Pass nil for db when no datastore is
// wired yet; readiness then reports ok without probing.
func NewHealth(db Pinger) *Health { return &Health{db: db} }

// Live answers /healthz: the process is up and serving. It never touches
// dependencies, so an overloaded database cannot make the service look dead to
// the supervisor and trigger a needless restart.
func (h *Health) Live(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Ready answers /readyz: the process can serve real traffic, which means its
// datastore is reachable. A load balancer/tunnel uses this to decide whether to
// route requests here.
func (h *Health) Ready(w http.ResponseWriter, r *http.Request) {
	if h.db == nil {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready", "db": "unconfigured"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := h.db.Ping(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unready", "db": "unreachable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready", "db": "ok"})
}

// writeJSON is a tiny shared helper for the plain JSON responses these probes
// emit; richer error responses go through the problem package (phase 4).
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
