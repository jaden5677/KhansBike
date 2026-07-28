-- Full-text and typeahead search. The weighted vector is maintained by the
-- trigger in 00008. The blended ranking that also mixes trigram similarity
-- (ts_rank_cd * 0.7 + similarity * 0.3) is assembled as a hand-written pgx query
-- in internal/store/postgres/search_repo.go, because pg_trgm's similarity() is
-- an extension function sqlc does not model. These sqlc queries cover the
-- type-safe tsvector path and a simple typeahead.

-- name: SearchProducts :many
SELECT p.id, p.category_id, p.brand_id, p.name, p.slug, p.summary, p.description,
       p.status, p.is_featured, p.retail_price_is_public, p.attrs,
       p.created_at, p.updated_at, p.published_at,
       ts_rank_cd(p.search_vector, websearch_to_tsquery('english', sqlc.arg(q)))::float8 AS rank
FROM products p
WHERE p.status = 'active'
  AND p.search_vector @@ websearch_to_tsquery('english', sqlc.arg(q))
ORDER BY rank DESC, p.is_featured DESC, p.name ASC
LIMIT sqlc.arg(row_limit);

-- name: CountSearchProducts :one
SELECT count(*)
FROM products p
WHERE p.status = 'active'
  AND p.search_vector @@ websearch_to_tsquery('english', sqlc.arg(q));

-- name: SuggestProducts :many
-- Typeahead. A simple prefix/substring match kept in sqlc for type safety; the
-- trigram-ranked variant lives in the repo. Capped small for latency.
SELECT slug, name
FROM products
WHERE status = 'active' AND name ILIKE '%' || sqlc.arg(q) || '%'
ORDER BY name
LIMIT 8;

-- name: ListProductIDsByWheelSize :many
-- Backs GET /api/v1/fitment/{wheel_size}: every active product whose attrs
-- projection contains the given wheel_size option value, across all categories.
SELECT id, slug, name, category_id
FROM products
WHERE status = 'active'
  AND attrs @> jsonb_build_object('wheel_size', sqlc.arg(wheel_size)::text)
ORDER BY name;
