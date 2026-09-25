-- Categories: the taxonomy tree. path is an ltree materialised path; queries
-- cast it to text so the generated Go sees a string.

-- name: CreateCategory :one
INSERT INTO categories (id, parent_id, name, slug, path, position, description, hero_asset_id, is_active)
VALUES (sqlc.arg(id), sqlc.narg(parent_id), sqlc.arg(name), sqlc.arg(slug), sqlc.arg(path)::ltree,
        sqlc.arg(position), sqlc.narg(description), sqlc.narg(hero_asset_id), sqlc.arg(is_active))
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

-- name: CategoryIsVisible :one
-- A category is publicly visible only when it and every ancestor are active.
SELECT NOT EXISTS (
    SELECT 1
    FROM categories c
    JOIN categories anc ON anc.path @> c.path
    WHERE c.id = $1 AND NOT anc.is_active
) AS visible;

-- name: CategorySlugExists :one
SELECT EXISTS (SELECT 1 FROM categories WHERE slug = $1) AS taken;

-- name: UpdateCategory :one
-- Optimistic write: only applies while the row still has the updated_at the
-- caller read, so a concurrent edit makes this return no row.
UPDATE categories
SET parent_id = sqlc.narg(parent_id),
    name = sqlc.arg(name),
    slug = sqlc.arg(slug),
    position = sqlc.arg(position),
    description = sqlc.narg(description),
    hero_asset_id = sqlc.narg(hero_asset_id),
    is_active = sqlc.arg(is_active),
    updated_at = now()
WHERE id = sqlc.arg(id) AND updated_at = sqlc.arg(expected_updated_at)
RETURNING id, parent_id, name, slug, path::text AS path, position, description, hero_asset_id, is_active, created_at, updated_at;

-- name: MoveCategorySubtree :exec
-- Re-roots a category and all of its descendants from old_path to new_path.
-- subpath() cannot take the full length of a path, so the subtree root itself
-- is handled by the CASE.
UPDATE categories
SET path = CASE WHEN path = sqlc.arg(old_path)::ltree
                THEN sqlc.arg(new_path)::ltree
                ELSE sqlc.arg(new_path)::ltree || subpath(path, nlevel(sqlc.arg(old_path)::ltree)) END,
    updated_at = now()
WHERE path <@ sqlc.arg(old_path)::ltree;

-- name: DeleteCategory :execrows
DELETE FROM categories WHERE id = $1;
