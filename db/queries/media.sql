-- Media assets, renditions, and product/variant associations. Assets dedupe by
-- sha256; re-uploading identical bytes returns the existing row.

-- name: InsertAsset :one
-- The no-op DO UPDATE makes RETURNING yield the existing row on a duplicate
-- digest, so an identical re-upload resolves to the asset already stored.
INSERT INTO media_assets (id, sha256, original_filename, mime, byte_size, width, height, storage_key, status, uploaded_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
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

-- name: ListAssetsByIDs :many
SELECT id, sha256, original_filename, mime, byte_size, width, height, blurhash,
       dominant_hex, storage_key, status, failure_reason, uploaded_by, created_at
FROM media_assets
WHERE id = ANY(sqlc.arg(ids)::uuid[]);

-- name: MarkAssetProcessing :exec
UPDATE media_assets SET status = 'processing', failure_reason = NULL WHERE id = $1;

-- name: MarkAssetReady :exec
UPDATE media_assets
SET status = 'ready', width = $2, height = $3, blurhash = $4, dominant_hex = $5, failure_reason = NULL
WHERE id = $1;

-- name: MarkAssetFailed :exec
UPDATE media_assets SET status = 'failed', failure_reason = $2 WHERE id = $1;

-- name: InsertRendition :one
INSERT INTO media_renditions (id, asset_id, width, height, format, storage_key, byte_size)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (asset_id, width, format)
DO UPDATE SET height = EXCLUDED.height, storage_key = EXCLUDED.storage_key, byte_size = EXCLUDED.byte_size
RETURNING id, asset_id, width, height, format, storage_key, byte_size;

-- name: ListRenditionsByAssets :many
SELECT id, asset_id, width, height, format, storage_key, byte_size
FROM media_renditions
WHERE asset_id = ANY(sqlc.arg(asset_ids)::uuid[])
ORDER BY asset_id, width, format;

-- name: AttachProductMedia :one
INSERT INTO product_media (id, product_id, variant_id, asset_id, role, position, alt_text)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, product_id, variant_id, asset_id, role, position, alt_text;

-- name: UpdateProductMedia :one
UPDATE product_media
SET variant_id = $3, role = $4, position = $5, alt_text = $6
WHERE id = $1 AND product_id = $2
RETURNING id, product_id, variant_id, asset_id, role, position, alt_text;

-- name: DeleteProductMedia :execrows
DELETE FROM product_media WHERE id = $1 AND product_id = $2;

-- name: ListProductMedia :many
SELECT pm.id, pm.product_id, pm.variant_id, pm.asset_id, pm.role, pm.position, pm.alt_text
FROM product_media pm
WHERE pm.product_id = $1
ORDER BY pm.position, pm.id;

-- name: ListCoverMediaByProducts :many
-- One cover image per product for listing cards: product-level images before
-- variant swatches, the hero before the gallery, then the admin's ordering.
-- Only processed (ready) assets can be shown.
SELECT DISTINCT ON (pm.product_id)
       pm.id, pm.product_id, pm.variant_id, pm.asset_id, pm.role, pm.position, pm.alt_text
FROM product_media pm
JOIN media_assets a ON a.id = pm.asset_id AND a.status = 'ready'
WHERE pm.product_id = ANY(sqlc.arg(product_ids)::uuid[])
ORDER BY pm.product_id, (pm.variant_id IS NULL) DESC, (pm.role = 'hero') DESC, pm.position, pm.id;
