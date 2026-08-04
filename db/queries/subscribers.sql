-- Mailing-list subscribers (double opt-in). Raw tokens live only in the emailed
-- links; these queries deal exclusively in their sha256 hashes.

-- name: UpsertSubscriber :one
-- Idempotent signup. A brand-new address is inserted as 'pending'. A previously
-- unsubscribed (or still-pending) address is reset to 'pending' with a fresh
-- confirmation token, so someone can always re-subscribe. An already-confirmed
-- address is left untouched (its status stays 'confirmed').
INSERT INTO mailing_list_subscribers
    (id, email, name, status, confirm_token_hash, confirm_expires_at, unsubscribe_token_hash, source)
VALUES ($1, $2, $3, 'pending', $4, $5, $6, $7)
ON CONFLICT (email) DO UPDATE SET
    name = COALESCE(EXCLUDED.name, mailing_list_subscribers.name),
    status = CASE WHEN mailing_list_subscribers.status = 'confirmed'
                  THEN 'confirmed'::subscriber_status
                  ELSE 'pending'::subscriber_status END,
    confirm_token_hash = CASE WHEN mailing_list_subscribers.status = 'confirmed'
                              THEN mailing_list_subscribers.confirm_token_hash
                              ELSE EXCLUDED.confirm_token_hash END,
    confirm_expires_at = CASE WHEN mailing_list_subscribers.status = 'confirmed'
                              THEN mailing_list_subscribers.confirm_expires_at
                              ELSE EXCLUDED.confirm_expires_at END,
    updated_at = now()
RETURNING id, email, name, status, confirm_token_hash, confirm_expires_at,
          unsubscribe_token_hash, source, confirmed_at, unsubscribed_at, created_at, updated_at;

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
    updated_at = now()
WHERE unsubscribe_token_hash = $1 AND status <> 'unsubscribed'
RETURNING id, email, status;

-- name: GetSubscriberByEmail :one
SELECT id, email, name, status, confirm_token_hash, confirm_expires_at,
       unsubscribe_token_hash, source, confirmed_at, unsubscribed_at, created_at, updated_at
FROM mailing_list_subscribers
WHERE email = $1;

-- name: ListConfirmedSubscribers :many
-- ADMIN ONLY. Keyset pagination over (created_at, id) for CSV export.
SELECT id, email, name, source, confirmed_at, created_at
FROM mailing_list_subscribers
WHERE status = 'confirmed'
  AND (created_at, id) > (sqlc.arg(after_created_at), sqlc.arg(after_id))
ORDER BY created_at, id
LIMIT sqlc.arg(row_limit);

-- name: CountSubscribersByStatus :many
SELECT status, count(*) AS total
FROM mailing_list_subscribers
GROUP BY status;
