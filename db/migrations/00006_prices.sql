-- 00006_prices: one row per (variant, tier, effective_from). Money is stored as
-- integer minor units (amount_minor) plus an ISO currency code; never a float.
-- Three of the four tiers are admin-only; the public read path filters on tier
-- and never joins non-retail rows.
-- +goose Up
CREATE TYPE price_tier AS ENUM ('cost_usd', 'landed_ttd', 'wholesale_ttd', 'retail_ttd');

CREATE TABLE prices (
    id             uuid PRIMARY KEY,
    variant_id     uuid NOT NULL REFERENCES product_variants(id) ON DELETE CASCADE,
    tier           price_tier NOT NULL,
    amount_minor   bigint NOT NULL,                  -- integer cents; exact
    currency       char(3) NOT NULL,                 -- 'USD' | 'TTD'
    effective_from date NOT NULL DEFAULT current_date,
    source_note    text,
    created_at     timestamptz NOT NULL DEFAULT now(),
    UNIQUE (variant_id, tier, effective_from)
);
-- Composite index so "current retail price for this variant" is a cheap lookup.
CREATE INDEX ix_prices_variant_tier ON prices (variant_id, tier, effective_from DESC);

-- +goose Down
DROP TABLE IF EXISTS prices;
DROP TYPE IF EXISTS price_tier;
