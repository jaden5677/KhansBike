-- Products, variants, and typed attribute values. The attrs JSONB column is the
-- fast filter path and is rewritten by the service in the same transaction as
-- the EAV rows; these queries expose both.

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

-- name: UpdateProductAttrs :exec
UPDATE products SET attrs = $2, updated_at = now() WHERE id = $1;

-- name: SetProductStatus :exec
UPDATE products SET status = $2, published_at = $3, updated_at = now() WHERE id = $1;

-- name: ProductSlugExists :one
SELECT EXISTS (SELECT 1 FROM products WHERE slug = $1) AS taken;

-- name: ListActiveProductsByCategory :many
-- Keyset (cursor) pagination over a stable (created_at, id) order. The cursor is
-- the last row's (created_at, id); the first page passes the epoch and a nil UUID.
SELECT id, category_id, brand_id, name, slug, summary, description, status,
       is_featured, retail_price_is_public, attrs, created_at, updated_at, published_at
FROM products
WHERE category_id = $1
  AND status = 'active'
  AND (created_at, id) > (sqlc.arg(after_created_at), sqlc.arg(after_id))
ORDER BY created_at, id
LIMIT sqlc.arg(row_limit);

-- name: CountActiveProductsByCategory :one
SELECT count(*) FROM products WHERE category_id = $1 AND status = 'active';

-- name: CreateVariant :one
INSERT INTO product_variants (id, product_id, sku, supplier_id, supplier_item_no, model_no,
                              name_suffix, position, stock_status, attrs, is_default)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING id, product_id, sku, supplier_id, supplier_item_no, model_no, name_suffix,
          position, stock_status, attrs, is_default, created_at, updated_at;

-- name: ListVariantsByProduct :many
SELECT id, product_id, sku, supplier_id, supplier_item_no, model_no, name_suffix,
       position, stock_status, attrs, is_default, created_at, updated_at
FROM product_variants
WHERE product_id = $1
ORDER BY position, id;

-- name: UpdateVariantAttrs :exec
UPDATE product_variants SET attrs = $2, updated_at = now() WHERE id = $1;

-- name: InsertAttributeValue :one
INSERT INTO product_attribute_values (id, product_id, variant_id, attribute_id, option_id,
                                      value_text, value_num, value_num_low, value_num_high, value_bool, etrto)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING id;

-- name: ListAttributeValuesByProduct :many
SELECT id, product_id, variant_id, attribute_id, option_id,
       value_text, value_num, value_num_low, value_num_high, value_bool, etrto
FROM product_attribute_values
WHERE product_id = $1;

-- name: DeleteAttributeValuesForProduct :exec
DELETE FROM product_attribute_values WHERE product_id = $1;
