-- Attributes, their options, and the per-category bindings that drive the
-- server-side form schema.

-- name: CreateAttribute :one
INSERT INTO attributes (id, key, label, data_type, unit, input_type, is_filterable, is_searchable, help_text)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING id, key, label, data_type, unit, input_type, is_filterable, is_searchable, help_text, created_at, updated_at;

-- name: GetAttributeByKey :one
SELECT id, key, label, data_type, unit, input_type, is_filterable, is_searchable, help_text, created_at, updated_at
FROM attributes
WHERE key = $1;

-- name: GetAttributeByID :one
SELECT id, key, label, data_type, unit, input_type, is_filterable, is_searchable, help_text, created_at, updated_at
FROM attributes
WHERE id = $1;

-- name: ListAttributes :many
SELECT id, key, label, data_type, unit, input_type, is_filterable, is_searchable, help_text, created_at, updated_at
FROM attributes
ORDER BY key;

-- name: UpdateAttribute :one
-- key and data_type are deliberately not updatable: stored values and filter
-- URLs depend on them. Optimistic on updated_at like the other admin writes.
UPDATE attributes
SET label = sqlc.arg(label),
    unit = sqlc.narg(unit),
    input_type = sqlc.arg(input_type),
    is_filterable = sqlc.arg(is_filterable),
    is_searchable = sqlc.arg(is_searchable),
    help_text = sqlc.narg(help_text),
    updated_at = now()
WHERE id = sqlc.arg(id) AND updated_at = sqlc.arg(expected_updated_at)
RETURNING id, key, label, data_type, unit, input_type, is_filterable, is_searchable, help_text, created_at, updated_at;

-- name: TouchAttribute :exec
-- Bumps the attribute's version when one of its options changes, so clients
-- holding the attribute (or a form schema derived from it) see a new ETag.
UPDATE attributes SET updated_at = now() WHERE id = $1;

-- name: DeleteAttribute :execrows
DELETE FROM attributes WHERE id = $1;

-- name: CreateAttributeOption :one
INSERT INTO attribute_options (id, attribute_id, value, label, swatch_hex, position)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, attribute_id, value, label, swatch_hex, position;

-- name: UpdateAttributeOption :one
-- value is the stable token stored in product projections and filter URLs, so
-- only the presentation fields change.
UPDATE attribute_options
SET label = $3, swatch_hex = $4, position = $5
WHERE id = $1 AND attribute_id = $2
RETURNING id, attribute_id, value, label, swatch_hex, position;

-- name: DeleteAttributeOption :execrows
DELETE FROM attribute_options WHERE id = $1 AND attribute_id = $2;

-- name: ListOptionsByAttribute :many
SELECT id, attribute_id, value, label, swatch_hex, position
FROM attribute_options
WHERE attribute_id = $1
ORDER BY position, value;

-- name: ListOptionsByAttributeIDs :many
SELECT id, attribute_id, value, label, swatch_hex, position
FROM attribute_options
WHERE attribute_id = ANY(sqlc.arg(attribute_ids)::uuid[])
ORDER BY attribute_id, position, value;

-- name: BindCategoryAttribute :exec
INSERT INTO category_attributes (category_id, attribute_id, position, is_required, is_variant_axis, label_override)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (category_id, attribute_id)
DO UPDATE SET position = EXCLUDED.position,
              is_required = EXCLUDED.is_required,
              is_variant_axis = EXCLUDED.is_variant_axis,
              label_override = EXCLUDED.label_override;

-- name: UnbindCategoryAttribute :execrows
DELETE FROM category_attributes WHERE category_id = $1 AND attribute_id = $2;

-- name: CategoryAttributeHasValues :one
-- Whether any product in the category stores a value for the attribute.
SELECT EXISTS (
    SELECT 1
    FROM product_attribute_values pav
    JOIN products p ON p.id = pav.product_id
    WHERE p.category_id = $1 AND pav.attribute_id = $2
) AS has_values;

-- name: DeleteCategoryAttributeValues :many
-- Removes an attribute's values from every product in a category (when the
-- attribute is unbound there) and returns the affected product ids so their
-- JSONB projections can be rebuilt in the same transaction.
DELETE FROM product_attribute_values pav
USING products p
WHERE pav.product_id = p.id
  AND p.category_id = sqlc.arg(category_id)
  AND pav.attribute_id = sqlc.arg(attribute_id)
RETURNING pav.product_id;

-- name: ListCategoryAttributes :many
-- Joined view that feeds the form schema: the binding plus the attribute it
-- points at, ordered as the admin arranged them.
SELECT ca.category_id, ca.attribute_id, ca.position, ca.is_required, ca.is_variant_axis, ca.label_override,
       a.key, a.label, a.data_type, a.unit, a.input_type, a.is_filterable, a.is_searchable, a.help_text,
       a.created_at, a.updated_at
FROM category_attributes ca
JOIN attributes a ON a.id = ca.attribute_id
WHERE ca.category_id = $1
ORDER BY ca.position, a.key;
