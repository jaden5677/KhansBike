-- 00005_products: products, their variants, and the typed EAV value rows, plus
-- the denormalised JSONB attribute projection (attrs) used as the fast filter
-- path. The EAV rows are the source of truth and enforce referential integrity;
-- attrs is rebuilt in the same transaction and indexed with GIN jsonb_path_ops.
-- +goose Up
CREATE TYPE product_status AS ENUM ('draft', 'active', 'discontinued', 'needs_review');
CREATE TYPE stock_status   AS ENUM ('in_stock', 'low', 'out', 'special_order', 'unknown');

CREATE TABLE products (
    id                     uuid PRIMARY KEY,
    category_id            uuid NOT NULL REFERENCES categories(id) ON DELETE RESTRICT,
    brand_id               uuid REFERENCES brands(id) ON DELETE SET NULL,
    name                   text NOT NULL,
    slug                   text UNIQUE NOT NULL,
    summary                text,
    description            text,
    status                 product_status NOT NULL DEFAULT 'draft',
    is_featured            boolean NOT NULL DEFAULT false,
    retail_price_is_public boolean NOT NULL DEFAULT false,  -- gate for the only publicly-visible price tier
    attrs                  jsonb NOT NULL DEFAULT '{}',     -- projection of product-level EAV values
    search_vector          tsvector,                        -- maintained by trigger in 00008
    created_at             timestamptz NOT NULL DEFAULT now(),
    updated_at             timestamptz NOT NULL DEFAULT now(),
    published_at           timestamptz
);
CREATE INDEX ix_products_category ON products (category_id);
CREATE INDEX ix_products_brand ON products (brand_id);
CREATE INDEX ix_products_status ON products (status);
CREATE INDEX ix_products_attrs ON products USING gin (attrs jsonb_path_ops);   -- facet filter path
CREATE INDEX ix_products_name_trgm ON products USING gin (name gin_trgm_ops);  -- typo tolerance
-- Keyset pagination support: stable (created_at, id) ordering.
CREATE INDEX ix_products_keyset ON products (created_at, id);

CREATE TABLE product_variants (
    id               uuid PRIMARY KEY,
    product_id       uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    sku              text NOT NULL,
    supplier_id      uuid REFERENCES suppliers(id) ON DELETE SET NULL,  -- ADMIN ONLY
    supplier_item_no text,                                              -- ADMIN ONLY
    model_no         text,
    name_suffix      text,                                             -- e.g. 'Blue / 9/16"'
    position         int NOT NULL DEFAULT 0,
    stock_status     stock_status NOT NULL DEFAULT 'unknown',
    attrs            jsonb NOT NULL DEFAULT '{}',
    is_default       boolean NOT NULL DEFAULT false,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    UNIQUE (product_id, sku)
);
CREATE INDEX ix_variants_product ON product_variants (product_id);
CREATE INDEX ix_variants_attrs ON product_variants USING gin (attrs jsonb_path_ops);
-- Exactly one default variant per product.
CREATE UNIQUE INDEX ux_variant_default ON product_variants (product_id) WHERE is_default;

CREATE TABLE product_attribute_values (
    id             uuid PRIMARY KEY,
    product_id     uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    variant_id     uuid REFERENCES product_variants(id) ON DELETE CASCADE,  -- NULL => product-level
    attribute_id   uuid NOT NULL REFERENCES attributes(id) ON DELETE RESTRICT,
    option_id      uuid REFERENCES attribute_options(id) ON DELETE RESTRICT,
    value_text     text,
    value_num      double precision,
    value_num_low  double precision,
    value_num_high double precision,
    value_bool     boolean,
    etrto          text,                                   -- reserved; ETRTO not implemented in v1
    -- Exactly one logical value slot populated; a number_range counts low+high as one slot.
    CONSTRAINT one_value CHECK (
        ( (value_text IS NOT NULL)::int
        + (value_num  IS NOT NULL)::int
        + (value_bool IS NOT NULL)::int
        + (option_id  IS NOT NULL)::int
        + ((value_num_low IS NOT NULL OR value_num_high IS NOT NULL))::int ) = 1
    ),
    CONSTRAINT range_is_pair CHECK ((value_num_low IS NULL) = (value_num_high IS NULL))
);
CREATE INDEX ix_pav_product ON product_attribute_values (product_id);
CREATE INDEX ix_pav_variant ON product_attribute_values (variant_id);
CREATE INDEX ix_pav_attr ON product_attribute_values (attribute_id);
CREATE INDEX ix_pav_option ON product_attribute_values (option_id);
-- Numeric range overlap filter (tyre/tube width): "20 inch, 2.0 wide" matches 1.95/2.125.
CREATE INDEX ix_pav_numrange ON product_attribute_values
    USING gist (numrange(value_num_low::numeric, value_num_high::numeric, '[]'))
    WHERE value_num_low IS NOT NULL;

-- +goose Down
DROP TABLE IF EXISTS product_attribute_values;
DROP TABLE IF EXISTS product_variants;
DROP TABLE IF EXISTS products;
DROP TYPE IF EXISTS stock_status;
DROP TYPE IF EXISTS product_status;
