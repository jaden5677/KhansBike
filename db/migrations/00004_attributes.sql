-- 00004_attributes: the dynamic attribute system that lets categories gain new
-- fields entirely from the admin UI. attributes is the global registry;
-- attribute_options holds enum-like choices; category_attributes is the binding
-- that makes an attribute apply to a category (with per-binding required /
-- variant-axis / label-override). This is the heart of the app.
-- +goose Up
CREATE TYPE attr_data_type AS ENUM
    ('text', 'number', 'number_range', 'boolean', 'enum', 'multi_enum', 'color');

CREATE TABLE attributes (
    id            uuid PRIMARY KEY,
    key           text UNIQUE NOT NULL,              -- immutable machine key, e.g. 'wheel_size'
    label         text NOT NULL,                     -- human label, editable
    data_type     attr_data_type NOT NULL,
    unit          text,                              -- 'in','mm','T','speed'
    input_type    text NOT NULL,                     -- UI hint: select|radio|swatch|range|checkbox|text
    is_filterable boolean NOT NULL DEFAULT true,
    is_searchable boolean NOT NULL DEFAULT false,
    help_text     text,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE attribute_options (
    id           uuid PRIMARY KEY,
    attribute_id uuid NOT NULL REFERENCES attributes(id) ON DELETE CASCADE,
    value        text NOT NULL,                      -- canonical token, e.g. '20'
    label        text NOT NULL,                      -- display, e.g. '20"'
    swatch_hex   text,                               -- colour chip for color attributes
    position     int NOT NULL DEFAULT 0,
    UNIQUE (attribute_id, value)
);
CREATE INDEX ix_attr_options_attr ON attribute_options (attribute_id);

CREATE TABLE category_attributes (
    category_id     uuid NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
    attribute_id    uuid NOT NULL REFERENCES attributes(id) ON DELETE RESTRICT,
    position        int NOT NULL DEFAULT 0,
    is_required     boolean NOT NULL DEFAULT false,
    is_variant_axis boolean NOT NULL DEFAULT false,  -- true => differentiates variants (colour/thread/size)
    label_override  text,
    PRIMARY KEY (category_id, attribute_id)
);
CREATE INDEX ix_cat_attrs_attr ON category_attributes (attribute_id);

-- +goose Down
DROP TABLE IF EXISTS category_attributes;
DROP TABLE IF EXISTS attribute_options;
DROP TABLE IF EXISTS attributes;
DROP TYPE IF EXISTS attr_data_type;
