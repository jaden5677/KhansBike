package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/khansbikezone/bikezone-api/internal/domain"
	"github.com/khansbikezone/bikezone-api/internal/store"
	"github.com/khansbikezone/bikezone-api/internal/store/gen"
)

// ErrInvalidCredentials is deliberately vague: it does not reveal whether
// the email exists, the password was wrong, or the account is locked out.
var ErrInvalidCredentials = fmt.Errorf("%w: invalid email or password, or the account is temporarily locked", domain.ErrUnauthorized)

var errInvalidSession = fmt.Errorf("%w: missing, invalid or expired credentials", domain.ErrUnauthorized)

// Lockout policy: after lockoutThreshold consecutive failures the account is
// locked for lockoutBase, doubling with each further failure up to
// lockoutMax. This throttles online guessing without a permanent lockout that
// an attacker could use to deny the owner access.
const (
	lockoutThreshold = 5
	lockoutBase      = time.Minute
	lockoutMax       = time.Hour
)

// lockoutFor returns how long to lock an account that has now failed
// failures times in a row (zero means do not lock).
func lockoutFor(failures int) time.Duration {
	if failures < lockoutThreshold {
		return 0
	}
	d := lockoutBase
	for i := lockoutThreshold; i < failures && d < lockoutMax; i++ {
		d *= 2
	}
	return min(d, lockoutMax)
}

const (
	pairingCodeTTL = 10 * time.Minute
	// touchInterval throttles last_seen_at writes to one per credential per
	// interval instead of one per request.
	touchInterval = 5 * time.Minute
	maxUserAgent  = 512
	maxDeviceName = 100
)

// Service authenticates admins and their devices.
type Service struct {
	store      *store.Store
	csrfKey    []byte
	sessionTTL time.Duration
	log        *slog.Logger
	now        func() time.Time
}

// NewService builds the auth service. csrfKey is the 32-byte CSRF_KEY.
func NewService(st *store.Store, csrfKey []byte, sessionTTL time.Duration, log *slog.Logger) *Service {
	return &Service{store: st, csrfKey: csrfKey, sessionTTL: sessionTTL, log: log, now: time.Now}
}

// dummyHash is verified against when the email is unknown, so a login for a
// missing account costs the same time as one with a wrong password and the
// response time does not reveal which emails exist.
var dummyHash = sync.OnceValue(func() string {
	h, _ := HashPassword("timing-equaliser-not-a-real-password")
	return h
})

// NewSession is the result of a successful login. Token is the raw cookie
// value; it is shown to the client exactly once and never stored.
type NewSession struct {
	Token     string
	ExpiresAt time.Time
	CSRFToken string
	Principal *Principal
}

// Login verifies an email and password and opens a browser session.
func (s *Service) Login(ctx context.Context, email, password, userAgent string, ip netip.Addr) (*NewSession, error) {
	user, err := s.store.GetUserByEmail(ctx, strings.TrimSpace(email))
	if errors.Is(err, domain.ErrNotFound) {
		_, _ = VerifyPassword(dummyHash(), password)
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, fmt.Errorf("load user: %w", err)
	}
	now := s.now()
	if user.LockedUntil.Valid && user.LockedUntil.Time.After(now) {
		s.log.WarnContext(ctx, "login rejected: account locked", "user_id", user.ID, "until", user.LockedUntil.Time)
		return nil, ErrInvalidCredentials
	}
	ok, err := VerifyPassword(user.PasswordHash, password)
	if err != nil {
		return nil, fmt.Errorf("verify password for user %s: %w", user.ID, err)
	}
	if !ok {
		failures := int(user.FailedLoginCount) + 1
		var until *time.Time
		if d := lockoutFor(failures); d > 0 {
			t := now.Add(d)
			until = &t
			s.log.WarnContext(ctx, "account locked after failed logins", "user_id", user.ID, "failures", failures, "until", t)
		}
		if err := s.store.RecordFailedLogin(ctx, gen.RecordFailedLoginParams{ID: user.ID, LockedUntil: store.NullTimestamptz(until)}); err != nil {
			return nil, fmt.Errorf("record failed login: %w", err)
		}
		return nil, ErrInvalidCredentials
	}

	token, err := NewToken()
	if err != nil {
		return nil, err
	}
	expires := now.Add(s.sessionTTL)
	principal := &Principal{
		UserID:      user.ID,
		Email:       user.Email,
		DisplayName: user.DisplayName,
		Role:        domain.Role(user.Role),
		Kind:        domain.ActorAdmin,
		SessionID:   domain.NewID(),
	}
	err = s.store.InTx(ctx, func(q *store.Queries) error {
		if user.FailedLoginCount > 0 || user.LockedUntil.Valid {
			if err := q.ResetFailedLogin(ctx, user.ID); err != nil {
				return err
			}
		}
		// Transparently upgrade hashes made with older argon2 parameters.
		if needsRehash(user.PasswordHash) {
			h, err := HashPassword(password)
			if err != nil {
				return err
			}
			if _, err := q.UpdateUserPassword(ctx, gen.UpdateUserPasswordParams{ID: user.ID, PasswordHash: h}); err != nil {
				return err
			}
		}
		params := gen.CreateSessionParams{
			ID:        principal.SessionID,
			UserID:    user.ID,
			TokenHash: HashToken(token),
			UserAgent: store.TextOrNull(truncate(userAgent, maxUserAgent)),
			ExpiresAt: store.Timestamptz(expires),
		}
		if ip.IsValid() {
			params.Ip = &ip
		}
		if _, err := q.CreateSession(ctx, params); err != nil {
			return err
		}
		actx := domain.WithActor(ctx, domain.Actor{UserID: &user.ID, Kind: domain.ActorAdmin, IP: ip})
		return q.Audit(actx, "auth.login", "user", &user.ID, nil, nil)
	})
	if err != nil {
		return nil, fmt.Errorf("open session: %w", err)
	}
	return &NewSession{
		Token:     token,
		ExpiresAt: expires,
		CSRFToken: CSRFToken(s.csrfKey, principal.SessionID),
		Principal: principal,
	}, nil
}

// Logout ends a browser session.
func (s *Service) Logout(ctx context.Context, p *Principal) error {
	if !p.IsSession() {
		return fmt.Errorf("%w: devices are signed out by revoking them", domain.ErrForbidden)
	}
	return s.store.DeleteSession(ctx, p.SessionID)
}

// AuthenticateSession resolves a session cookie value to its principal.
func (s *Service) AuthenticateSession(ctx context.Context, token string) (*Principal, error) {
	row, err := s.store.GetSessionByTokenHash(ctx, HashToken(token))
	if errors.Is(err, domain.ErrNotFound) {
		return nil, errInvalidSession
	}
	if err != nil {
		return nil, fmt.Errorf("load session: %w", err)
	}
	if s.now().Sub(row.LastSeenAt.Time) > touchInterval {
		if err := s.store.TouchSession(ctx, row.ID); err != nil {
			s.log.WarnContext(ctx, "touch session failed", "session_id", row.ID, "error", err)
		}
	}
	return &Principal{
		UserID:      row.UserID,
		Email:       row.Email,
		DisplayName: row.DisplayName,
		Role:        domain.Role(row.Role),
		Kind:        domain.ActorAdmin,
		SessionID:   row.ID,
	}, nil
}

// AuthenticateDevice resolves a device bearer token to its principal.
func (s *Service) AuthenticateDevice(ctx context.Context, token string) (*Principal, error) {
	row, err := s.store.GetDeviceTokenByHash(ctx, HashToken(token))
	if errors.Is(err, domain.ErrNotFound) {
		return nil, errInvalidSession
	}
	if err != nil {
		return nil, fmt.Errorf("load device: %w", err)
	}
	if !row.LastSeenAt.Valid || s.now().Sub(row.LastSeenAt.Time) > touchInterval {
		if err := s.store.TouchDeviceToken(ctx, row.ID); err != nil {
			s.log.WarnContext(ctx, "touch device failed", "device_id", row.ID, "error", err)
		}
	}
	return &Principal{
		UserID:      row.UserID,
		Email:       row.Email,
		DisplayName: row.DisplayName,
		Role:        domain.Role(row.Role),
		Kind:        domain.ActorDevice,
		DeviceID:    row.ID,
	}, nil
}

// CSRFToken returns the CSRF token for a session principal.
func (s *Service) CSRFToken(p *Principal) string { return CSRFToken(s.csrfKey, p.SessionID) }

// VerifyCSRF checks a session principal's CSRF token.
func (s *Service) VerifyCSRF(p *Principal, token string) bool {
	return token != "" && VerifyCSRFToken(s.csrfKey, p.SessionID, token)
}

// ChangePassword replaces the caller's password after checking the current
// one, and signs out every other session.
func (s *Service) ChangePassword(ctx context.Context, p *Principal, current, next string) error {
	if !p.IsSession() {
		return fmt.Errorf("%w: passwords can only be changed from a browser session", domain.ErrForbidden)
	}
	user, err := s.store.GetUserByID(ctx, p.UserID)
	if err != nil {
		return fmt.Errorf("load user: %w", err)
	}
	ok, err := VerifyPassword(user.PasswordHash, current)
	if err != nil {
		return fmt.Errorf("verify password: %w", err)
	}
	if !ok {
		return domain.Invalid("currentPassword", "is incorrect")
	}
	if err := ValidatePassword(next); err != nil {
		return domain.Invalid("newPassword", "%s", err)
	}
	hash, err := HashPassword(next)
	if err != nil {
		return err
	}
	return s.store.InTx(ctx, func(q *store.Queries) error {
		if _, err := q.UpdateUserPassword(ctx, gen.UpdateUserPasswordParams{ID: p.UserID, PasswordHash: hash}); err != nil {
			return err
		}
		if err := q.DeleteOtherSessions(ctx, gen.DeleteOtherSessionsParams{UserID: p.UserID, KeepSessionID: p.SessionID}); err != nil {
			return err
		}
		return q.Audit(ctx, "user.password_change", "user", &p.UserID, nil, nil)
	})
}

// CreatePairingCode issues a short-lived single-use code (shown as a QR) that
// the owner's phone redeems for a device token.
func (s *Service) CreatePairingCode(ctx context.Context, p *Principal) (domain.PairingCode, error) {
	if !p.IsSession() {
		// A stolen phone must not be able to mint credentials for more phones.
		return domain.PairingCode{}, fmt.Errorf("%w: devices can only be paired from a browser session", domain.ErrForbidden)
	}
	expires := s.now().Add(pairingCodeTTL)
	for attempt := 0; ; attempt++ {
		code, err := newPairingCode()
		if err != nil {
			return domain.PairingCode{}, err
		}
		row, err := s.store.CreatePairingCode(ctx, gen.CreatePairingCodeParams{
			Code: code, UserID: p.UserID, ExpiresAt: store.Timestamptz(expires),
		})
		if errors.Is(err, domain.ErrConflict) && attempt < 3 {
			continue // astronomically unlikely collision with a live code
		}
		if err != nil {
			return domain.PairingCode{}, fmt.Errorf("create pairing code: %w", err)
		}
		return domain.PairingCode{Code: row.Code, UserID: row.UserID, ExpiresAt: row.ExpiresAt.Time}, nil
	}
}

// RedeemPairingCode exchanges a pairing code for a device token. The raw
// token is returned exactly once; the server keeps only its hash. ip is the
// redeeming client's address, for the audit log.
func (s *Service) RedeemPairingCode(ctx context.Context, code, deviceName string, ip netip.Addr) (string, domain.DeviceToken, error) {
	name := truncate(strings.TrimSpace(deviceName), maxDeviceName)
	if name == "" {
		name = "Paired device"
	}
	token, err := NewToken()
	if err != nil {
		return "", domain.DeviceToken{}, err
	}
	var device domain.DeviceToken
	err = s.store.InTx(ctx, func(q *store.Queries) error {
		pc, err := q.ConsumePairingCode(ctx, normalizePairingCode(code))
		if errors.Is(err, domain.ErrNotFound) {
			return fmt.Errorf("%w: the pairing code is invalid, expired or already used", domain.ErrUnauthorized)
		}
		if err != nil {
			return err
		}
		row, err := q.CreateDeviceToken(ctx, gen.CreateDeviceTokenParams{
			ID: domain.NewID(), UserID: pc.UserID, Name: name, TokenHash: HashToken(token),
		})
		if err != nil {
			return err
		}
		device = domain.DeviceToken{ID: row.ID, UserID: row.UserID, Name: row.Name, CreatedAt: row.CreatedAt.Time}
		actx := domain.WithActor(ctx, domain.Actor{UserID: &pc.UserID, Kind: domain.ActorDevice, IP: ip})
		return q.Audit(actx, "device.pair", "device", &device.ID, nil, map[string]string{"name": name})
	})
	if err != nil {
		return "", domain.DeviceToken{}, err
	}
	return token, device, nil
}

// ListDevices lists a user's paired devices, including revoked ones.
func (s *Service) ListDevices(ctx context.Context, userID uuid.UUID) ([]domain.DeviceToken, error) {
	rows, err := s.store.ListDeviceTokensByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list devices: %w", err)
	}
	out := make([]domain.DeviceToken, len(rows))
	for i, r := range rows {
		out[i] = domain.DeviceToken{
			ID:         r.ID,
			UserID:     r.UserID,
			Name:       r.Name,
			LastSeenAt: store.TimePtr(r.LastSeenAt),
			RevokedAt:  store.TimePtr(r.RevokedAt),
			CreatedAt:  r.CreatedAt.Time,
		}
	}
	return out, nil
}

// RevokeDevice permanently disables one of the user's devices.
func (s *Service) RevokeDevice(ctx context.Context, userID, deviceID uuid.UUID) error {
	return s.store.InTx(ctx, func(q *store.Queries) error {
		n, err := q.RevokeDeviceToken(ctx, gen.RevokeDeviceTokenParams{ID: deviceID, UserID: userID})
		if err != nil {
			return err
		}
		if n == 0 {
			return fmt.Errorf("device %s: %w", deviceID, domain.ErrNotFound)
		}
		return q.Audit(ctx, "device.revoke", "device", &deviceID, nil, nil)
	})
}

// CreateUser registers a user. It backs the admin bootstrap CLI; there is no
// HTTP endpoint for it (v1 has a single owner account).
func (s *Service) CreateUser(ctx context.Context, email, displayName, password string, role domain.Role) (domain.User, error) {
	v := &domain.ValidationError{}
	addr, err := mail.ParseAddress(strings.TrimSpace(email))
	if err != nil || addr.Address != strings.TrimSpace(email) {
		v.Add("email", "is not a valid email address")
	}
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		v.Add("displayName", "is required")
	}
	if err := ValidatePassword(password); err != nil {
		v.Add("password", "%s", err)
	}
	if role != domain.RoleAdmin && role != domain.RoleWholesale {
		v.Add("role", "is not a known role")
	}
	if err := v.Err(); err != nil {
		return domain.User{}, err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return domain.User{}, err
	}
	row, err := s.store.CreateUser(ctx, gen.CreateUserParams{
		ID: domain.NewID(), Email: addr.Address, PasswordHash: hash, Role: gen.UserRole(role), DisplayName: displayName,
	})
	if err != nil {
		return domain.User{}, fmt.Errorf("create user: %w", err)
	}
	return domain.User{ID: row.ID, Email: row.Email, Role: domain.Role(row.Role), DisplayName: row.DisplayName, CreatedAt: row.CreatedAt.Time}, nil
}

// ResetPassword sets a user's password out of band (the admin CLI's recovery
// path), clears any lockout, and signs out all of their sessions.
func (s *Service) ResetPassword(ctx context.Context, email, password string) error {
	if err := ValidatePassword(password); err != nil {
		return domain.Invalid("password", "%s", err)
	}
	user, err := s.store.GetUserByEmail(ctx, strings.TrimSpace(email))
	if err != nil {
		return fmt.Errorf("user %q: %w", email, err)
	}
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	return s.store.InTx(ctx, func(q *store.Queries) error {
		if _, err := q.UpdateUserPassword(ctx, gen.UpdateUserPasswordParams{ID: user.ID, PasswordHash: hash}); err != nil {
			return err
		}
		if err := q.DeleteOtherSessions(ctx, gen.DeleteOtherSessionsParams{UserID: user.ID, KeepSessionID: uuid.Nil}); err != nil {
			return err
		}
		return q.Audit(ctx, "user.password_reset", "user", &user.ID, nil, nil)
	})
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	// Cut on a rune boundary so the result stays valid UTF-8.
	for n > 0 && n < len(s) && s[n]&0xC0 == 0x80 {
		n--
	}
	return s[:n]
}
