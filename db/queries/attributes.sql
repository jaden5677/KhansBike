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

-- name: CreateAttributeOption :one
INSERT INTO attribute_options (id, attribute_id, value, label, swatch_hex, position)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, attribute_id, value, label, swatch_hex, position;

-- name: ListOptionsByAttribute :many
SELECT id, attribute_id, value, label, swatch_hex, position
FROM attribute_options
WHERE attribute_id = $1
ORDER BY position, value;

-- name: GetOptionByValue :one
SELECT id, attribute_id, value, label, swatch_hex, position
FROM attribute_options
WHERE attribute_id = $1 AND value = $2;

-- name: BindCategoryAttribute :exec
INSERT INTO category_attributes (category_id, attribute_id, position, is_required, is_variant_axis, label_override)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (category_id, attribute_id)
DO UPDATE SET position = EXCLUDED.position,
              is_required = EXCLUDED.is_required,
              is_variant_axis = EXCLUDED.is_variant_axis,
              label_override = EXCLUDED.label_override;

-- name: UnbindCategoryAttribute :exec
DELETE FROM category_attributes WHERE category_id = $1 AND attribute_id = $2;

-- name: ListCategoryAttributes :many
-- Joined view that feeds the form schema: the binding plus the attribute it
-- points at, ordered as the admin arranged them.
SELECT ca.category_id, ca.attribute_id, ca.position, ca.is_required, ca.is_variant_axis, ca.label_override,
       a.key, a.label, a.data_type, a.unit, a.input_type, a.is_filterable, a.is_searchable, a.help_text
FROM category_attributes ca
JOIN attributes a ON a.id = ca.attribute_id
WHERE ca.category_id = $1
ORDER BY ca.position, a.key;
