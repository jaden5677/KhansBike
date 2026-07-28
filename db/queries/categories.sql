-- Categories: the taxonomy tree. path is an ltree materialised path; queries
-- cast it to text so the generated Go sees a string.

-- name: CreateCategory :one
INSERT INTO categories (id, parent_id, name, slug, path, position, description, is_active)
VALUES ($1, $2, $3, $4, $5::ltree, $6, $7, $8)
RETURNING id, parent_id, name, slug, path::text AS path, position, description, hero_asset_id, is_active, created_at, updated_at;

-- name: GetCategoryByID :one
SELECT id, parent_id, name, slug, path::text AS path, position, description, hero_asset_id, is_active, created_at, updated_at
FROM categories
WHERE id = $1;

-- name: GetCategoryBySlug :one
SELECT id, parent_id, name, slug, path::text AS path, position, description, hero_asset_id, is_active, created_at, updated_at
FROM categories
WHERE slug = $1;

-- name: ListCategories :many
SELECT id, parent_id, name, slug, path::text AS path, position, description, hero_asset_id, is_active, created_at, updated_at
FROM categories
ORDER BY path;

-- name: ListActiveCategoriesWithCounts :many
SELECT c.id, c.parent_id, c.name, c.slug, c.path::text AS path, c.position,
       c.description, c.hero_asset_id, c.is_active, c.created_at, c.updated_at,
       count(p.id) FILTER (WHERE p.status = 'active') AS product_count
FROM categories c
LEFT JOIN products p ON p.category_id = c.id
WHERE c.is_active
GROUP BY c.id
ORDER BY c.path;

-- name: SlugExists :one
SELECT EXISTS (SELECT 1 FROM categories WHERE slug = $1) AS taken;
