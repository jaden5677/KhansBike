-- Prices. The public read path only ever selects the retail tier; the admin path
-- selects all tiers. Keeping these as separate named queries makes the
-- visibility boundary explicit in the query layer, not just in Go.

-- name: UpsertPrice :one
INSERT INTO prices (id, variant_id, tier, amount_minor, currency, effective_from, source_note)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (variant_id, tier, effective_from)
DO UPDATE SET amount_minor = EXCLUDED.amount_minor,
              currency = EXCLUDED.currency,
              source_note = EXCLUDED.source_note
RETURNING id, variant_id, tier, amount_minor, currency, effective_from, source_note, created_at;

-- name: ListPricesByVariant :many
-- ADMIN ONLY: returns every tier. Never call this on a public path.
SELECT id, variant_id, tier, amount_minor, currency, effective_from, source_note, created_at
FROM prices
WHERE variant_id = $1
ORDER BY tier, effective_from DESC;

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
