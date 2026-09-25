package handler

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/khansbikezone/bikezone-api/internal/http/middleware"
	"github.com/khansbikezone/bikezone-api/internal/service"
)

// Subscribers serves the mailing list: public signup/confirm/unsubscribe and
// the admin statistics and export.
type Subscribers struct {
	base
	svc *service.Subscribers
}

// NewSubscribers builds the mailing-list handlers.
func NewSubscribers(log *slog.Logger, svc *service.Subscribers) *Subscribers {
	return &Subscribers{base: base{log: log}, svc: svc}
}

type tokenBody struct {
	Token string `json:"token"`
}

// Subscribe answers POST /subscribers. The response is the same whether the
// address is new, pending or already confirmed.
func (h *Subscribers) Subscribe(w http.ResponseWriter, r *http.Request) {
	var in service.SubscribeInput
	if !h.decode(w, r, &in) {
		return
	}
	if err := h.svc.Subscribe(r.Context(), in); err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{
		"status": "If the address can receive mail, a confirmation link is on its way.",
	})
}

// Confirm answers POST /subscribers/confirm with the emailed token.
func (h *Subscribers) Confirm(w http.ResponseWriter, r *http.Request) {
	var in tokenBody
	if !h.decode(w, r, &in) {
		return
	}
	if err := h.svc.Confirm(r.Context(), in.Token); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Unsubscribe answers POST /subscribers/unsubscribe with the emailed token.
func (h *Subscribers) Unsubscribe(w http.ResponseWriter, r *http.Request) {
	var in tokenBody
	if !h.decode(w, r, &in) {
		return
	}
	if err := h.svc.Unsubscribe(r.Context(), in.Token); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Stats answers GET /admin/subscribers/stats.
func (h *Subscribers) Stats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.svc.Stats(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

// Export answers GET /admin/subscribers/export with a CSV download of the
// confirmed list, streamed so it never has to fit in memory.
func (h *Subscribers) Export(w http.ResponseWriter, r *http.Request) {
	name := "subscribers-" + time.Now().UTC().Format(time.DateOnly) + ".csv"
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	if err := h.svc.ExportConfirmed(r.Context(), w); err != nil {
		// Headers (and possibly rows) are already sent, so the status cannot
		// change; the client sees a truncated file and the log has the cause.
		h.log.ErrorContext(r.Context(), "subscriber export failed",
			"request_id", middleware.RequestIDFromContext(r.Context()), "error", err)
	}
}
