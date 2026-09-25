-- Mailing-list subscribers (double opt-in). Raw tokens live only in the emailed
-- links; these queries deal exclusively in their sha256 hashes.

-- name: UpsertSubscriber :one
-- Idempotent signup. A brand-new address is inserted as 'pending'. A previously
-- unsubscribed (or still-pending) address is reset to 'pending' so someone can
-- always re-subscribe. An already-confirmed address is left confirmed. Tokens
-- are not issued here: the confirmation job issues fresh ones at send time, so
-- raw tokens never need to be stored anywhere.
INSERT INTO mailing_list_subscribers (id, email, name, status, source)
VALUES ($1, $2, $3, 'pending', $4)
ON CONFLICT (email) DO UPDATE SET
    name = COALESCE(EXCLUDED.name, mailing_list_subscribers.name),
    status = CASE WHEN mailing_list_subscribers.status = 'confirmed'
                  THEN 'confirmed'::subscriber_status
                  ELSE 'pending'::subscriber_status END,
    updated_at = now()
RETURNING id, status;

-- name: ClaimConfirmationSend :execrows
-- Claims the right to email a pending subscriber a confirmation link, unless
-- one was requested within the cooldown (its expiry is still later than
-- cooldown_until). It is a single UPDATE, so of several rapid or concurrent
-- signups for one address exactly one wins, and one email is sent. The
-- expiry is provisional: the send job replaces it when it issues the tokens.
UPDATE mailing_list_subscribers
SET confirm_expires_at = sqlc.arg(expires_at), updated_at = now()
WHERE id = sqlc.arg(id)
  AND status = 'pending'
  AND (confirm_expires_at IS NULL OR confirm_expires_at < sqlc.arg(cooldown_until));

-- name: IssueSubscriberTokens :one
-- Stores the hashes of a freshly generated confirm/unsubscribe token pair for a
-- still-pending subscriber and returns the address to email them to. No row
-- means the subscriber confirmed or left in the meantime: send nothing.
UPDATE mailing_list_subscribers
SET confirm_token_hash = $2,
    confirm_expires_at = $3,
    unsubscribe_token_hash = $4,
    updated_at = now()
WHERE id = $1 AND status = 'pending'
RETURNING email, name;

-- name: ConfirmSubscriber :one
-- Complete double opt-in: match the (unexpired) confirm token, flip to confirmed,
-- and clear the token so the link cannot be replayed.
UPDATE mailing_list_subscribers
SET status = 'confirmed',
    confirmed_at = now(),
    confirm_token_hash = NULL,
    confirm_expires_at = NULL,
    updated_at = now()
WHERE confirm_token_hash = $1
  AND status = 'pending'
  AND (confirm_expires_at IS NULL OR confirm_expires_at > now())
RETURNING id, email, name, status, confirmed_at;

-- name: UnsubscribeByToken :one
UPDATE mailing_list_subscribers
SET status = 'unsubscribed',
    unsubscribed_at = now(),
    confirm_token_hash = NULL,
    confirm_expires_at = NULL,
    updated_at = now()
WHERE unsubscribe_token_hash = $1 AND status <> 'unsubscribed'
RETURNING id, email, status;

-- name: ListConfirmedSubscribers :many
-- ADMIN ONLY. Keyset pagination over (created_at, id) for CSV export.
SELECT id, email, name, source, confirmed_at, created_at
FROM mailing_list_subscribers
WHERE status = 'confirmed'
  AND (created_at, id) > (sqlc.arg(after_created_at)::timestamptz, sqlc.arg(after_id)::uuid)
ORDER BY created_at, id
LIMIT sqlc.arg(row_limit);

-- name: CountSubscribersByStatus :many
SELECT status, count(*) AS total
FROM mailing_list_subscribers
GROUP BY status;
