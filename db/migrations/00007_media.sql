-- 00007_media: content-addressed image assets, their derived renditions, and the
-- product/variant associations. Assets dedupe by sha256. product_media uses a
-- surrogate primary key plus a unique index over a COALESCE expression, because
-- a PRIMARY KEY constraint cannot contain an expression (a variant_id of NULL
-- must still collapse to one uniqueness slot per product+asset).
-- +goose Up
CREATE TYPE asset_status AS ENUM ('pending', 'processing', 'ready', 'failed');
CREATE TYPE media_role   AS ENUM ('hero', 'gallery', 'detail', 'swatch');

CREATE TABLE media_assets (
    id                uuid PRIMARY KEY,
    sha256            bytea UNIQUE NOT NULL,          -- 32-byte content digest; identical bytes dedupe
    original_filename text NOT NULL,
    mime              text NOT NULL,
    byte_size         bigint NOT NULL,
    width             int,
    height            int,
    blurhash          text,
    dominant_hex      text,
    storage_key       text NOT NULL,
    status            asset_status NOT NULL DEFAULT 'pending',
    failure_reason    text,
    uploaded_by       uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at        timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE media_renditions (
    id          uuid PRIMARY KEY,
    asset_id    uuid NOT NULL REFERENCES media_assets(id) ON DELETE CASCADE,
    width       int NOT NULL,
    height      int NOT NULL,
    format      text NOT NULL,                        -- 'webp' | 'jpeg'
    storage_key text NOT NULL,
    byte_size   bigint NOT NULL,
    UNIQUE (asset_id, width, format)
);

CREATE TABLE product_media (
    id         uuid PRIMARY KEY,
    product_id uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    variant_id uuid REFERENCES product_variants(id) ON DELETE CASCADE,   -- NULL => product-level
    asset_id   uuid NOT NULL REFERENCES media_assets(id) ON DELETE RESTRICT,
    role       media_role NOT NULL DEFAULT 'gallery',
    position   int NOT NULL DEFAULT 0,
    alt_text   text
);
CREATE UNIQUE INDEX ux_product_media ON product_media
    (product_id, asset_id, COALESCE(variant_id, '00000000-0000-0000-0000-000000000000'::uuid));
CREATE INDEX ix_product_media_product ON product_media (product_id);

-- Deferred foreign keys from 00003 now that media_assets exists.
ALTER TABLE brands
    ADD CONSTRAINT fk_brand_logo FOREIGN KEY (logo_asset_id)
    REFERENCES media_assets(id) ON DELETE SET NULL;
ALTER TABLE categories
    ADD CONSTRAINT fk_cat_hero FOREIGN KEY (hero_asset_id)
    REFERENCES media_assets(id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE categories DROP CONSTRAINT IF EXISTS fk_cat_hero;
ALTER TABLE brands DROP CONSTRAINT IF EXISTS fk_brand_logo;
DROP TABLE IF EXISTS product_media;
DROP TABLE IF EXISTS media_renditions;
DROP TABLE IF EXISTS media_assets;
DROP TYPE IF EXISTS media_role;
DROP TYPE IF EXISTS asset_status;
