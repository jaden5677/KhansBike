-- Users, sessions, device tokens, and pairing codes. Password hashes and token
-- hashes are opaque to these queries; hashing happens in the auth package.

-- name: CreateUser :one
INSERT INTO users (id, email, password_hash, role, display_name)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, email, password_hash, role, display_name, failed_login_count, locked_until, created_at, updated_at;

-- name: GetUserByEmail :one
SELECT id, email, password_hash, role, display_name, failed_login_count, locked_until, created_at, updated_at
FROM users
WHERE email = $1;

-- name: GetUserByID :one
SELECT id, email, password_hash, role, display_name, failed_login_count, locked_until, created_at, updated_at
FROM users
WHERE id = $1;

-- name: UpdateUserPassword :execrows
UPDATE users
SET password_hash = $2,
    failed_login_count = 0,
    locked_until = NULL,
    updated_at = now()
WHERE id = $1;

-- name: RecordFailedLogin :exec
UPDATE users
SET failed_login_count = failed_login_count + 1,
    locked_until = $2,
    updated_at = now()
WHERE id = $1;

-- name: ResetFailedLogin :exec
UPDATE users
SET failed_login_count = 0,
    locked_until = NULL,
    updated_at = now()
WHERE id = $1;

-- name: CreateSession :one
INSERT INTO sessions (id, user_id, token_hash, user_agent, ip, expires_at)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, user_id, token_hash, user_agent, expires_at, created_at, last_seen_at;

-- name: GetSessionByTokenHash :one
-- Resolves a presented cookie token (by its hash) to the unexpired session and
-- the owning user's identity, in one round trip per authenticated request.
SELECT s.id, s.user_id, s.expires_at, s.last_seen_at, u.role, u.email, u.display_name
FROM sessions s
JOIN users u ON u.id = s.user_id
WHERE s.token_hash = $1 AND s.expires_at > now();

-- name: TouchSession :exec
UPDATE sessions SET last_seen_at = now() WHERE id = $1;

-- name: DeleteSession :exec
DELETE FROM sessions WHERE id = $1;

-- name: DeleteOtherSessions :exec
-- Signs a user out everywhere except the current session (after a password change).
DELETE FROM sessions WHERE user_id = $1 AND id <> sqlc.arg(keep_session_id);

-- name: DeleteExpiredSessions :execrows
DELETE FROM sessions WHERE expires_at <= now();

-- name: CreateDeviceToken :one
INSERT INTO device_tokens (id, user_id, name, token_hash)
VALUES ($1, $2, $3, $4)
RETURNING id, user_id, name, last_seen_at, revoked_at, created_at;

-- name: GetDeviceTokenByHash :one
-- Resolves a presented bearer token (by its hash) to the live device and the
-- owning user's identity.
SELECT d.id, d.user_id, d.name, d.last_seen_at, u.role, u.email, u.display_name
FROM device_tokens d
JOIN users u ON u.id = d.user_id
WHERE d.token_hash = $1 AND d.revoked_at IS NULL;

-- name: TouchDeviceToken :exec
UPDATE device_tokens SET last_seen_at = now() WHERE id = $1;

-- name: ListDeviceTokensByUser :many
SELECT id, user_id, name, last_seen_at, revoked_at, created_at
FROM device_tokens
WHERE user_id = $1
ORDER BY created_at DESC;

-- name: RevokeDeviceToken :execrows
UPDATE device_tokens
SET revoked_at = now()
WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL;

-- name: CreatePairingCode :one
INSERT INTO pairing_codes (code, user_id, expires_at)
VALUES ($1, $2, $3)
RETURNING code, user_id, expires_at, consumed_at;

-- name: ConsumePairingCode :one
UPDATE pairing_codes
SET consumed_at = now()
WHERE code = $1 AND consumed_at IS NULL AND expires_at > now()
RETURNING code, user_id, expires_at, consumed_at;

-- name: DeleteStalePairingCodes :execrows
DELETE FROM pairing_codes WHERE expires_at < now() - interval '1 day';
