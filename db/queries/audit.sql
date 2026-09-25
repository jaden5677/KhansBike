-- The audit log: an append-only record of every admin write. Rows are written
-- in the same transaction as the change they describe.

-- name: InsertAuditLog :exec
INSERT INTO audit_log (id, actor_user_id, actor_kind, action, entity_type, entity_id, before, after, ip)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: ListAuditLog :many
-- Newest first, keyset-paginated on (created_at, id), optionally narrowed to
-- one entity type and/or one entity.
SELECT l.id, l.actor_user_id, u.email AS actor_email, l.actor_kind, l.action, l.entity_type,
       l.entity_id, l.before, l.after, l.ip, l.created_at
FROM audit_log l
LEFT JOIN users u ON u.id = l.actor_user_id
WHERE (sqlc.narg(entity_type)::text IS NULL OR l.entity_type = sqlc.narg(entity_type)::text)
  AND (sqlc.narg(entity_id)::uuid IS NULL OR l.entity_id = sqlc.narg(entity_id)::uuid)
  AND (l.created_at, l.id) < (sqlc.arg(before_created_at)::timestamptz, sqlc.arg(before_id)::uuid)
ORDER BY l.created_at DESC, l.id DESC
LIMIT sqlc.arg(row_limit);
