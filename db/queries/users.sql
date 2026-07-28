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
SELECT id, user_id, token_hash, user_agent, expires_at, created_at, last_seen_at
FROM sessions
WHERE token_hash = $1 AND expires_at > now();

-- name: TouchSession :exec
UPDATE sessions SET last_seen_at = now() WHERE id = $1;

-- name: DeleteSession :exec
DELETE FROM sessions WHERE id = $1;

-- name: DeleteExpiredSessions :execrows
DELETE FROM sessions WHERE expires_at <= now();

-- name: CreateDeviceToken :one
INSERT INTO device_tokens (id, user_id, name, token_hash)
VALUES ($1, $2, $3, $4)
RETURNING id, user_id, name, token_hash, last_seen_at, revoked_at, created_at;

-- name: GetDeviceTokenByHash :one
SELECT id, user_id, name, token_hash, last_seen_at, revoked_at, created_at
FROM device_tokens
WHERE token_hash = $1 AND revoked_at IS NULL;

-- name: ListDeviceTokensByUser :many
SELECT id, user_id, name, token_hash, last_seen_at, revoked_at, created_at
FROM device_tokens
WHERE user_id = $1
ORDER BY created_at DESC;

-- name: RevokeDeviceToken :exec
UPDATE device_tokens SET revoked_at = now() WHERE id = $1;

-- name: CreatePairingCode :one
INSERT INTO pairing_codes (code, user_id, expires_at)
VALUES ($1, $2, $3)
RETURNING code, user_id, expires_at, consumed_at;

-- name: ConsumePairingCode :one
UPDATE pairing_codes
SET consumed_at = now()
WHERE code = $1 AND consumed_at IS NULL AND expires_at > now()
RETURNING code, user_id, expires_at, consumed_at;
