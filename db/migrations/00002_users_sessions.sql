-- 00002_users_sessions: admin identity plus the two authentication mechanisms
-- (browser sessions and mobile device tokens) and the QR pairing handshake.
-- Only the 'admin' role is issued in v1; 'wholesale' exists in the enum so a
-- future trade-login feature needs no type migration.
-- +goose Up
CREATE TYPE user_role AS ENUM ('admin', 'wholesale');

CREATE TABLE users (
    id                 uuid PRIMARY KEY,
    email              citext UNIQUE NOT NULL,
    password_hash      text NOT NULL,               -- argon2id encoded string; never a raw password
    role               user_role NOT NULL DEFAULT 'admin',
    display_name       text NOT NULL,
    failed_login_count int NOT NULL DEFAULT 0,       -- drives lockout backoff
    locked_until       timestamptz,                  -- non-null while temporarily locked
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE sessions (
    id           uuid PRIMARY KEY,
    user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash   bytea UNIQUE NOT NULL,              -- sha256 of the opaque cookie value; the raw token is never stored
    user_agent   text,
    ip           inet,
    expires_at   timestamptz NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    last_seen_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ix_sessions_user ON sessions (user_id);
CREATE INDEX ix_sessions_expires ON sessions (expires_at);  -- supports expiry sweeps

CREATE TABLE device_tokens (
    id           uuid PRIMARY KEY,
    user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name         text NOT NULL,                      -- human label, e.g. "Owner's iPhone"
    token_hash   bytea UNIQUE NOT NULL,
    last_seen_at timestamptz,
    revoked_at   timestamptz,                        -- non-null once the device is de-authorised
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ix_device_tokens_user ON device_tokens (user_id);

CREATE TABLE pairing_codes (
    code        text PRIMARY KEY,                    -- short, single-use code shown as a QR
    user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at  timestamptz NOT NULL,
    consumed_at timestamptz                          -- non-null once redeemed for a device token
);

-- +goose Down
DROP TABLE IF EXISTS pairing_codes;
DROP TABLE IF EXISTS device_tokens;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS users;
DROP TYPE IF EXISTS user_role;
