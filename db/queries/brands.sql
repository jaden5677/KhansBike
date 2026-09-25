-- Brands (public) and suppliers (ADMIN ONLY). Suppliers are competitively
-- sensitive: no public read path may call the supplier queries.

-- name: ListBrands :many
SELECT id, name, slug, logo_asset_id, position
FROM brands
ORDER BY position, name;

-- name: GetBrandByID :one
SELECT id, name, slug, logo_asset_id, position
FROM brands
WHERE id = $1;

-- name: GetBrandBySlug :one
SELECT id, name, slug, logo_asset_id, position
FROM brands
WHERE slug = $1;

-- name: BrandSlugExists :one
SELECT EXISTS (SELECT 1 FROM brands WHERE slug = $1) AS taken;

-- name: CreateBrand :one
INSERT INTO brands (id, name, slug, logo_asset_id, position)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, name, slug, logo_asset_id, position;

-- name: UpdateBrand :one
UPDATE brands
SET name = $2, slug = $3, logo_asset_id = $4, position = $5
WHERE id = $1
RETURNING id, name, slug, logo_asset_id, position;

-- name: DeleteBrand :execrows
DELETE FROM brands WHERE id = $1;

-- name: ListSuppliers :many
-- ADMIN ONLY.
SELECT id, name, code, notes
FROM suppliers
ORDER BY name;

-- name: GetSupplierByID :one
-- ADMIN ONLY.
SELECT id, name, code, notes
FROM suppliers
WHERE id = $1;

-- name: CreateSupplier :one
INSERT INTO suppliers (id, name, code, notes)
VALUES ($1, $2, $3, $4)
RETURNING id, name, code, notes;

-- name: UpdateSupplier :one
UPDATE suppliers
SET name = $2, code = $3, notes = $4
WHERE id = $1
RETURNING id, name, code, notes;

-- name: DeleteSupplier :execrows
DELETE FROM suppliers WHERE id = $1;
