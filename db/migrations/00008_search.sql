-- 00008_search: the weighted tsvector maintenance for products. Weights: product
-- name / brand / sku / supplier_item_no = A, summary / category = B, enum option
-- labels = C, description = D. 'simple' config is used for names, skus, and
-- option labels (no stemming — do not stem a brand like "Kenda"); 'english' is
-- used only for prose. f_unaccent folds diacritics for accent-insensitive match.
-- +goose Up
-- +goose StatementBegin
CREATE FUNCTION products_refresh_search_vector(p_id uuid) RETURNS void
LANGUAGE plpgsql AS $$
DECLARE
    v tsvector;
BEGIN
    SELECT
          setweight(to_tsvector('simple',  f_unaccent(coalesce(p.name, ''))), 'A')
        || setweight(to_tsvector('simple',  f_unaccent(coalesce(b.name, ''))), 'A')
        || setweight(to_tsvector('simple',  f_unaccent(coalesce(string_agg(DISTINCT pv.sku, ' '), ''))), 'A')
        || setweight(to_tsvector('simple',  f_unaccent(coalesce(string_agg(DISTINCT pv.supplier_item_no, ' '), ''))), 'A')
        || setweight(to_tsvector('english', f_unaccent(coalesce(p.summary, ''))), 'B')
        || setweight(to_tsvector('simple',  f_unaccent(coalesce(c.name, ''))), 'B')
        || setweight(to_tsvector('simple',  f_unaccent(coalesce(string_agg(DISTINCT o.label, ' '), ''))), 'C')
        || setweight(to_tsvector('english', f_unaccent(coalesce(p.description, ''))), 'D')
    INTO v
    FROM products p
    LEFT JOIN brands b ON b.id = p.brand_id
    LEFT JOIN categories c ON c.id = p.category_id
    LEFT JOIN product_variants pv ON pv.product_id = p.id
    LEFT JOIN product_attribute_values pav ON pav.product_id = p.id
    LEFT JOIN attribute_options o ON o.id = pav.option_id
    WHERE p.id = p_id
    GROUP BY p.name, b.name, p.summary, c.name, p.description;

    UPDATE products SET search_vector = v WHERE id = p_id;
END;
$$;
-- +goose StatementEnd

CREATE INDEX ix_products_search ON products USING gin (search_vector);

-- Row trigger for the cheap product-local fields, kept synchronous so a rename
-- is immediately searchable.
-- +goose StatementBegin
CREATE FUNCTION trg_products_search() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    PERFORM products_refresh_search_vector(NEW.id);
    RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER products_search_vector
    AFTER INSERT OR UPDATE OF name, summary, description, brand_id, category_id
    ON products
    FOR EACH ROW EXECUTE FUNCTION trg_products_search();

-- +goose Down
DROP TRIGGER IF EXISTS products_search_vector ON products;
DROP FUNCTION IF EXISTS trg_products_search();
DROP INDEX IF EXISTS ix_products_search;
DROP FUNCTION IF EXISTS products_refresh_search_vector(uuid);
