package handler

import (
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/khansbikezone/bikezone-api/internal/auth"
	"github.com/khansbikezone/bikezone-api/internal/http/dto"
	"github.com/khansbikezone/bikezone-api/internal/http/middleware"
)

// CookieConfig describes the session cookie.
type CookieConfig struct {
	Name   string
	Secure bool // HTTPS only; true in production
}

// Auth serves sign-in, the current session, password changes and device
// pairing.
type Auth struct {
	base
	svc           *auth.Service
	cookie        CookieConfig
	publicBaseURL string
}

// NewAuth builds the authentication handlers.
func NewAuth(log *slog.Logger, svc *auth.Service, cookie CookieConfig, publicBaseURL string) *Auth {
	return &Auth{base: base{log: log}, svc: svc, cookie: cookie, publicBaseURL: strings.TrimSuffix(publicBaseURL, "/")}
}

// setCookie writes the session cookie. HttpOnly keeps it away from scripts;
// SameSite=Strict keeps other sites' requests from carrying it; the /api path
// keeps it off static asset requests.
func (h *Auth) setCookie(w http.ResponseWriter, value string, expires time.Time) {
	c := &http.Cookie{
		Name: h.cookie.Name, Value: value, Path: "/api", HttpOnly: true, Secure: h.cookie.Secure,
		SameSite: http.SameSiteStrictMode, Expires: expires,
	}
	if value == "" {
		c.MaxAge = -1 // delete
	}
	http.SetCookie(w, c)
}

// Login answers POST /auth/login.
func (h *Auth) Login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !h.decode(w, r, &in) {
		return
	}
	s, err := h.svc.Login(r.Context(), in.Email, in.Password, r.UserAgent(), middleware.ClientIPFrom(r.Context()))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.setCookie(w, s.Token, s.ExpiresAt)
	writeJSON(w, http.StatusOK, dto.NewSession(s.Principal, s.CSRFToken, &s.ExpiresAt))
}

// Logout answers POST /auth/logout.
func (h *Auth) Logout(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Logout(r.Context(), auth.PrincipalFrom(r.Context())); err != nil {
		h.fail(w, r, err)
		return
	}
	h.setCookie(w, "", time.Time{})
	w.WriteHeader(http.StatusNoContent)
}

// Session answers GET /auth/session: who the caller is, and (for a browser)
// the CSRF token to send with writes.
func (h *Auth) Session(w http.ResponseWriter, r *http.Request) {
	p := auth.PrincipalFrom(r.Context())
	csrf := ""
	if p.IsSession() {
		csrf = h.svc.CSRFToken(p)
	}
	writeJSON(w, http.StatusOK, dto.NewSession(p, csrf, nil))
}

// ChangePassword answers PUT /auth/password.
func (h *Auth) ChangePassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if !h.decode(w, r, &in) {
		return
	}
	if err := h.svc.ChangePassword(r.Context(), auth.PrincipalFrom(r.Context()), in.CurrentPassword, in.NewPassword); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Pair answers POST /auth/devices: a phone redeems a pairing code for its
// bearer token. The token is in this response only.
func (h *Auth) Pair(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Code string `json:"code"`
		Name string `json:"name"`
	}
	if !h.decode(w, r, &in) {
		return
	}
	token, device, err := h.svc.RedeemPairingCode(r.Context(), in.Code, in.Name, middleware.ClientIPFrom(r.Context()))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, dto.PairedDevice{Token: token, Device: dto.NewDevice(device)})
}

// CreatePairingCode answers POST /admin/devices/pairing-codes. The web admin
// renders URL as a QR code; the phone app reads the server address and the
// code from it.
func (h *Auth) CreatePairingCode(w http.ResponseWriter, r *http.Request) {
	pc, err := h.svc.CreatePairingCode(r.Context(), auth.PrincipalFrom(r.Context()))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, dto.PairingCode{
		Code:      pc.Code,
		URL:       h.publicBaseURL + "/pair?code=" + url.QueryEscape(pc.Code),
		ExpiresAt: pc.ExpiresAt,
	})
}

// Devices answers GET /admin/devices.
func (h *Auth) Devices(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.ListDevices(r.Context(), auth.PrincipalFrom(r.Context()).UserID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := make([]dto.Device, len(list))
	for i, d := range list {
		out[i] = dto.NewDevice(d)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

// RevokeDevice answers DELETE /admin/devices/{id}.
func (h *Auth) RevokeDevice(w http.ResponseWriter, r *http.Request) {
	id, ok := h.idParam(w, r, "id")
	if !ok {
		return
	}
	if err := h.svc.RevokeDevice(r.Context(), auth.PrincipalFrom(r.Context()).UserID, id); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
