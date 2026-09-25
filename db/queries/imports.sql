-- The xlsx importer's staging area: a batch per uploaded workbook and one row
-- per source row, each carrying the normalised proposal, detected issues, and
-- the reviewer's decision. Rows are ordered by id: ids are UUIDv7 minted in
-- workbook order, so id order is source order.

-- name: CreateImportBatch :one
INSERT INTO import_batches (id, filename, sha256, status, created_by)
VALUES ($1, $2, $3, 'dry_run', $4)
RETURNING id, filename, sha256, status, created_by, created_at, committed_at;

-- name: GetImportBatch :one
SELECT id, filename, sha256, status, created_by, created_at, committed_at
FROM import_batches
WHERE id = $1;

-- name: ListImportBatches :many
SELECT id, filename, sha256, status, created_by, created_at, committed_at
FROM import_batches
ORDER BY created_at DESC
LIMIT $1;

-- name: SetImportBatchStatus :execrows
-- Moves a batch to a new state only from one of the expected states, so two
-- concurrent commits (or a commit racing an abort) cannot both succeed.
UPDATE import_batches
SET status = sqlc.arg(to_status)::text,
    committed_at = CASE WHEN sqlc.arg(to_status)::text = 'committed' THEN now() ELSE committed_at END
WHERE id = sqlc.arg(id) AND status = ANY(sqlc.arg(from_statuses)::text[]);

-- name: InsertImportRows :copyfrom
INSERT INTO import_rows (id, batch_id, sheet_name, row_index, raw, proposed, issues, decision, target_product_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: ListImportRows :many
-- Review page: keyset over id, optionally narrowed to one decision and/or to
-- rows that have at least one detected issue.
SELECT id, batch_id, sheet_name, row_index, raw, proposed, issues, decision, target_product_id
FROM import_rows
WHERE batch_id = sqlc.arg(batch_id)
  AND (sqlc.narg(decision)::import_decision IS NULL OR decision = sqlc.narg(decision)::import_decision)
  AND (NOT sqlc.arg(with_issues_only)::bool OR jsonb_array_length(issues) > 0)
  AND id > sqlc.arg(after_id)::uuid
ORDER BY id
LIMIT sqlc.arg(row_limit);

-- name: GetImportRow :one
SELECT id, batch_id, sheet_name, row_index, raw, proposed, issues, decision, target_product_id
FROM import_rows
WHERE id = $1 AND batch_id = $2;

-- name: ListAllImportRows :many
SELECT id, batch_id, sheet_name, row_index, raw, proposed, issues, decision, target_product_id
FROM import_rows
WHERE batch_id = $1
ORDER BY id;

-- name: CountImportRowsByDecision :many
SELECT decision, count(*) AS total
FROM import_rows
WHERE batch_id = $1
GROUP BY decision;

-- name: SetImportRowDecision :one
UPDATE import_rows
SET decision = $3
WHERE id = $1 AND batch_id = $2
RETURNING id, batch_id, sheet_name, row_index, raw, proposed, issues, decision, target_product_id;

-- name: SetImportRowTarget :exec
UPDATE import_rows SET target_product_id = $2 WHERE id = $1;
