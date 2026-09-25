package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/khansbikezone/bikezone-api/internal/auth"
	"github.com/khansbikezone/bikezone-api/internal/domain"
	"github.com/khansbikezone/bikezone-api/internal/http/problem"
)

// CSRFHeader carries the anti-CSRF token on state-changing requests made
// with a session cookie.
const CSRFHeader = "X-CSRF-Token"

// Authenticate resolves the request's credential and rejects the request (401)
// when there is none or it is not valid. The owner's phone sends its device
// token as "Authorization: Bearer ..."; the browser sends the session cookie.
//
// A cookie is sent by the browser automatically, even on a request another
// site triggers, so cookie-authenticated requests that change state must also
// carry the session's CSRF token in X-CSRF-Token, which only this site's own
// pages can read. A bearer token is never sent automatically and needs none.
//
// On success the principal and the audit actor are attached to the context.
func Authenticate(svc *auth.Service, cookieName string, log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			var (
				p   *auth.Principal
				err error
			)
			if token, ok := bearerToken(r); ok {
				p, err = svc.AuthenticateDevice(ctx, token)
			} else if c, cerr := r.Cookie(cookieName); cerr == nil && c.Value != "" {
				p, err = svc.AuthenticateSession(ctx, c.Value)
			} else {
				err = fmt.Errorf("%w: sign in to continue", domain.ErrUnauthorized)
			}
			if err != nil && ctx.Err() != nil {
				// The client gave up while we looked up its session: not a
				// server error, and nobody is waiting for an answer.
				w.WriteHeader(problem.StatusClientClosedRequest)
				return
			}
			if err != nil {
				pr := problem.FromError(err)
				if pr.Status >= http.StatusInternalServerError {
					log.ErrorContext(ctx, "authentication failed", "request_id", RequestIDFromContext(ctx), "error", err)
				}
				problem.Write(w, pr)
				return
			}
			if p.IsSession() && !isSafeMethod(r.Method) && !svc.VerifyCSRF(p, r.Header.Get(CSRFHeader)) {
				problem.Write(w, problem.New(http.StatusForbidden, "Missing or invalid CSRF token."))
				return
			}
			ctx = auth.WithPrincipal(ctx, p)
			ctx = domain.WithActor(ctx, domain.Actor{UserID: &p.UserID, Kind: p.Kind, IP: ClientIPFrom(ctx)})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireRole answers 403 unless the authenticated principal has role. It
// must run after Authenticate.
func RequireRole(role domain.Role) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if p := auth.PrincipalFrom(r.Context()); p == nil || p.Role != role {
				problem.Write(w, problem.New(http.StatusForbidden, "You do not have access to this resource."))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireSession answers 403 for a paired device: some operations (pairing
// further devices, managing them) are reserved for the browser session, so a
// lost phone cannot be used to mint more credentials. It must run after
// Authenticate.
func RequireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if p := auth.PrincipalFrom(r.Context()); p == nil || !p.IsSession() {
			problem.Write(w, problem.New(http.StatusForbidden, "This action is only available from a signed-in browser."))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func bearerToken(r *http.Request) (string, bool) {
	scheme, token, ok := strings.Cut(r.Header.Get("Authorization"), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") || strings.TrimSpace(token) == "" {
		return "", false
	}
	return strings.TrimSpace(token), true
}

func isSafeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
}
