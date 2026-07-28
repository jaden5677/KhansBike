-- Media assets, renditions, and product/variant associations. Assets dedupe by
-- sha256; re-uploading identical bytes returns the existing row.

-- name: InsertAsset :one
INSERT INTO media_assets (id, sha256, original_filename, mime, byte_size, storage_key, status, uploaded_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (sha256) DO UPDATE SET original_filename = media_assets.original_filename
RETURNING id, sha256, original_filename, mime, byte_size, width, height, blurhash,
          dominant_hex, storage_key, status, failure_reason, uploaded_by, created_at;

-- name: GetAssetByID :one
SELECT id, sha256, original_filename, mime, byte_size, width, height, blurhash,
       dominant_hex, storage_key, status, failure_reason, uploaded_by, created_at
FROM media_assets
WHERE id = $1;

-- name: GetAssetBySHA :one
SELECT id, sha256, original_filename, mime, byte_size, width, height, blurhash,
       dominant_hex, storage_key, status, failure_reason, uploaded_by, created_at
FROM media_assets
WHERE sha256 = $1;

-- name: MarkAssetReady :exec
UPDATE media_assets
SET status = 'ready', width = $2, height = $3, blurhash = $4, dominant_hex = $5
WHERE id = $1;

-- name: MarkAssetFailed :exec
UPDATE media_assets SET status = 'failed', failure_reason = $2 WHERE id = $1;

-- name: InsertRendition :one
INSERT INTO media_renditions (id, asset_id, width, height, format, storage_key, byte_size)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (asset_id, width, format)
DO UPDATE SET height = EXCLUDED.height, storage_key = EXCLUDED.storage_key, byte_size = EXCLUDED.byte_size
RETURNING id, asset_id, width, height, format, storage_key, byte_size;

-- name: ListRenditionsByAsset :many
SELECT id, asset_id, width, height, format, storage_key, byte_size
FROM media_renditions
WHERE asset_id = $1
ORDER BY width, format;

-- name: AttachProductMedia :one
INSERT INTO product_media (id, product_id, variant_id, asset_id, role, position, alt_text)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, product_id, variant_id, asset_id, role, position, alt_text;

-- name: ListProductMedia :many
SELECT pm.id, pm.product_id, pm.variant_id, pm.asset_id, pm.role, pm.position, pm.alt_text
FROM product_media pm
WHERE pm.product_id = $1
ORDER BY pm.position, pm.id;
