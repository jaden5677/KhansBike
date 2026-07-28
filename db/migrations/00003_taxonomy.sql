-- 00003_taxonomy: brands, suppliers, and the category tree. suppliers is
-- ADMIN-ONLY data (competitively sensitive) and no public read path ever joins
-- it. Category hierarchy uses ltree so "everything under Bikes" is a single
-- indexed query. logo_asset_id / hero_asset_id reference media_assets, which is
-- created later in 00007, so those foreign keys are added there.
-- +goose Up
CREATE TABLE brands (
    id            uuid PRIMARY KEY,
    name          text NOT NULL,
    slug          text UNIQUE NOT NULL,
    logo_asset_id uuid,                              -- FK -> media_assets added in 00007
    position      int NOT NULL DEFAULT 0
);

-- Supplier identity is never exposed publicly; it lives here only for admin use.
CREATE TABLE suppliers (
    id    uuid PRIMARY KEY,
    name  text NOT NULL,
    code  text,
    notes text
);

CREATE TABLE categories (
    id            uuid PRIMARY KEY,
    parent_id     uuid REFERENCES categories(id) ON DELETE RESTRICT,
    name          text NOT NULL,
    slug          text UNIQUE NOT NULL,
    path          ltree NOT NULL,                    -- materialised path, e.g. 'bikes.tyres'
    position      int NOT NULL DEFAULT 0,
    description   text,
    hero_asset_id uuid,                              -- FK -> media_assets added in 00007
    is_active     boolean NOT NULL DEFAULT true,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ix_categories_path ON categories USING gist (path); -- ancestor/descendant queries
CREATE INDEX ix_categories_parent ON categories (parent_id);

-- +goose Down
DROP TABLE IF EXISTS categories;
DROP TABLE IF EXISTS suppliers;
DROP TABLE IF EXISTS brands;
