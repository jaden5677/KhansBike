-- Products, variants, and typed attribute values. The attrs JSONB column is the
-- fast filter path and is rewritten by the service in the same transaction as
-- the EAV rows; these queries expose both. Filtered listings and search are
-- dynamic and live in internal/store/catalog.go.

-- name: CreateProduct :one
INSERT INTO products (id, category_id, brand_id, name, slug, summary, description,
                      status, is_featured, retail_price_is_public, attrs, published_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
RETURNING id, category_id, brand_id, name, slug, summary, description, status,
          is_featured, retail_price_is_public, attrs, created_at, updated_at, published_at;

-- name: GetProductByID :one
SELECT id, category_id, brand_id, name, slug, summary, description, status,
       is_featured, retail_price_is_public, attrs, created_at, updated_at, published_at
FROM products
WHERE id = $1;

-- name: GetProductBySlug :one
SELECT id, category_id, brand_id, name, slug, summary, description, status,
       is_featured, retail_price_is_public, attrs, created_at, updated_at, published_at
FROM products
WHERE slug = $1;

-- name: UpdateProduct :one
-- Optimistic write: applies only while updated_at still matches what the
-- service read, so a concurrent save turns this into "no row" (a 412).
UPDATE products
SET category_id = sqlc.arg(category_id),
    brand_id = sqlc.narg(brand_id),
    name = sqlc.arg(name),
    slug = sqlc.arg(slug),
    summary = sqlc.narg(summary),
    description = sqlc.narg(description),
    status = sqlc.arg(status),
    is_featured = sqlc.arg(is_featured),
    retail_price_is_public = sqlc.arg(retail_price_is_public),
    attrs = sqlc.arg(attrs),
    published_at = sqlc.narg(published_at),
    updated_at = now()
WHERE id = sqlc.arg(id) AND updated_at = sqlc.arg(expected_updated_at)
RETURNING id, category_id, brand_id, name, slug, summary, description, status,
          is_featured, retail_price_is_public, attrs, created_at, updated_at, published_at;

-- name: UpdateProductAttrs :exec
UPDATE products SET attrs = $2, updated_at = now() WHERE id = $1;

-- name: TouchProduct :exec
-- Bumps the product's version when a related row (e.g. media) changes.
UPDATE products SET updated_at = now() WHERE id = $1;

-- name: DeleteProduct :execrows
DELETE FROM products WHERE id = $1;

-- name: ProductSlugExists :one
SELECT EXISTS (SELECT 1 FROM products WHERE slug = $1) AS taken;

-- name: CreateVariant :one
INSERT INTO product_variants (id, product_id, sku, supplier_id, supplier_item_no, model_no,
                              name_suffix, position, stock_status, attrs, is_default)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING id, product_id, sku, supplier_id, supplier_item_no, model_no, name_suffix,
          position, stock_status, attrs, is_default, created_at, updated_at;

-- name: UpdateVariant :one
UPDATE product_variants
SET sku = $3, supplier_id = $4, supplier_item_no = $5, model_no = $6, name_suffix = $7,
    position = $8, stock_status = $9, attrs = $10, is_default = $11, updated_at = now()
WHERE id = $1 AND product_id = $2
RETURNING id, product_id, sku, supplier_id, supplier_item_no, model_no, name_suffix,
          position, stock_status, attrs, is_default, created_at, updated_at;

-- name: UpdateVariantStock :exec
UPDATE product_variants SET stock_status = $2, updated_at = now() WHERE id = $1;

-- name: ClearDefaultVariant :exec
-- Run before re-assigning the default so the one-default-per-product unique
-- index never sees two defaults mid-update.
UPDATE product_variants SET is_default = false WHERE product_id = $1 AND is_default;

-- name: DeleteVariantsExcept :exec
-- Removes the variants of a product that a full-document save no longer lists.
DELETE FROM product_variants
WHERE product_id = sqlc.arg(product_id) AND NOT (id = ANY(sqlc.arg(keep_ids)::uuid[]));

-- name: ListVariantsByProduct :many
SELECT id, product_id, sku, supplier_id, supplier_item_no, model_no, name_suffix,
       position, stock_status, attrs, is_default, created_at, updated_at
FROM product_variants
WHERE product_id = $1
ORDER BY position, id;

-- name: ListVariantsByCodes :many
-- ADMIN/IMPORT ONLY: finds existing variants whose SKU or supplier item number
-- matches any of the given codes, so an import can update instead of duplicate.
SELECT v.id, v.product_id, v.sku, v.supplier_item_no
FROM product_variants v
WHERE v.sku = ANY(sqlc.arg(codes)::text[])
   OR v.supplier_item_no = ANY(sqlc.arg(codes)::text[]);

-- name: UpdateVariantAttrs :exec
UPDATE product_variants SET attrs = $2, updated_at = now() WHERE id = $1;

-- name: InsertAttributeValues :copyfrom
-- Bulk insert via COPY: one round trip (and one statement-level reindex
-- trigger) per product save instead of one per value.
INSERT INTO product_attribute_values (id, product_id, variant_id, attribute_id, option_id,
                                      value_text, value_num, value_num_low, value_num_high, value_bool, etrto)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11);

-- name: ListAttributeValuesByProduct :many
SELECT id, product_id, variant_id, attribute_id, option_id,
       value_text, value_num, value_num_low, value_num_high, value_bool, etrto
FROM product_attribute_values
WHERE product_id = $1;

-- name: DeleteAttributeValuesForProduct :exec
DELETE FROM product_attribute_values WHERE product_id = $1;

-- name: ListVariantStockByProducts :many
-- Listing cards summarise availability across a product's variants.
SELECT product_id, stock_status
FROM product_variants
WHERE product_id = ANY(sqlc.arg(product_ids)::uuid[]);
