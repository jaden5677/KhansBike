-- 00011_mailing_list: the optional customer mailing list. This is the only
-- customer-facing write in the system (customers otherwise have no accounts).
-- Double opt-in: a signup lands as 'pending' with a hashed confirmation token
-- and is only counted once the emailed link is clicked ('confirmed'). Tokens are
-- stored hashed (sha256), never in clear, so a database leak yields no usable
-- confirm/unsubscribe links.
-- +goose Up
CREATE TYPE subscriber_status AS ENUM ('pending', 'confirmed', 'unsubscribed');

CREATE TABLE mailing_list_subscribers (
    id                    uuid PRIMARY KEY,
    email                 citext UNIQUE NOT NULL,
    name                  text,                          -- optional display name
    status                subscriber_status NOT NULL DEFAULT 'pending',
    confirm_token_hash    bytea,                         -- sha256 of the emailed confirm token; cleared on confirm
    confirm_expires_at    timestamptz,                   -- confirmation link validity window
    unsubscribe_token_hash bytea,                        -- sha256 of the persistent one-click unsubscribe token
    source                text,                          -- where they signed up, e.g. 'footer','product_page'
    confirmed_at          timestamptz,
    unsubscribed_at       timestamptz,
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ix_subscribers_status ON mailing_list_subscribers (status);
-- Token lookups are by hash; partial unique indexes keep them fast and collision-free.
CREATE UNIQUE INDEX ux_subscribers_confirm_token ON mailing_list_subscribers (confirm_token_hash)
    WHERE confirm_token_hash IS NOT NULL;
CREATE UNIQUE INDEX ux_subscribers_unsub_token ON mailing_list_subscribers (unsubscribe_token_hash)
    WHERE unsubscribe_token_hash IS NOT NULL;

-- +goose Down
DROP TABLE IF EXISTS mailing_list_subscribers;
DROP TYPE IF EXISTS subscriber_status;
