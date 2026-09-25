-- Prices. The public read path only ever selects the retail tier; the admin path
-- selects all tiers. Keeping these as separate named queries makes the
-- visibility boundary explicit in the query layer, not just in Go.

-- name: UpsertPrice :one
-- effective_from defaults to the database's current_date: the same clock the
-- "current price" reads use, so a price set late in the evening cannot land
-- on a date the reads consider to be in the future.
INSERT INTO prices (id, variant_id, tier, amount_minor, currency, effective_from, source_note)
VALUES (sqlc.arg(id), sqlc.arg(variant_id), sqlc.arg(tier), sqlc.arg(amount_minor), sqlc.arg(currency),
        coalesce(sqlc.narg(effective_from)::date, current_date), sqlc.narg(source_note))
ON CONFLICT (variant_id, tier, effective_from)
DO UPDATE SET amount_minor = EXCLUDED.amount_minor,
              currency = EXCLUDED.currency,
              source_note = EXCLUDED.source_note
RETURNING id, variant_id, tier, amount_minor, currency, effective_from, source_note, created_at;

-- name: ListCurrentPricesByProduct :many
-- ADMIN ONLY: the current (latest effective, not future-dated) price of every
-- tier for every variant of a product. Never call this on a public path.
SELECT DISTINCT ON (pr.variant_id, pr.tier)
       pr.id, pr.variant_id, pr.tier, pr.amount_minor, pr.currency, pr.effective_from, pr.source_note, pr.created_at
FROM prices pr
JOIN product_variants v ON v.id = pr.variant_id
WHERE v.product_id = $1
  AND pr.effective_from <= current_date
ORDER BY pr.variant_id, pr.tier, pr.effective_from DESC;

-- name: ListCurrentRetailPricesByProduct :many
-- PUBLIC-SAFE: only the retail tier, only current effective rows, and only when
-- the product opts in via retail_price_is_public.
SELECT DISTINCT ON (pr.variant_id)
       pr.id, pr.variant_id, pr.tier, pr.amount_minor, pr.currency, pr.effective_from, pr.source_note, pr.created_at
FROM prices pr
JOIN product_variants v ON v.id = pr.variant_id
JOIN products p ON p.id = v.product_id
WHERE p.id = $1
  AND p.retail_price_is_public
  AND pr.tier = 'retail_ttd'
  AND pr.effective_from <= current_date
ORDER BY pr.variant_id, pr.effective_from DESC;

-- name: ListCurrentRetailPricesByProducts :many
-- PUBLIC-SAFE batch form of ListCurrentRetailPricesByProduct for listing cards:
-- the same retail-tier-only, opted-in-only rule across many products at once.
SELECT DISTINCT ON (pr.variant_id)
       v.product_id, pr.variant_id, pr.amount_minor, pr.currency
FROM prices pr
JOIN product_variants v ON v.id = pr.variant_id
JOIN products p ON p.id = v.product_id
WHERE p.id = ANY(sqlc.arg(product_ids)::uuid[])
  AND p.retail_price_is_public
  AND pr.tier = 'retail_ttd'
  AND pr.effective_from <= current_date
ORDER BY pr.variant_id, pr.effective_from DESC;
