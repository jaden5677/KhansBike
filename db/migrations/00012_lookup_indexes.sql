-- 00012_lookup_indexes: indexes for the access paths added with the service
-- layer. The importer and admin search look variants up by SKU or supplier
-- item number across all products (the existing unique index leads with
-- product_id, so it cannot serve that); the admin product list sorts by recent
-- edits; the audit log is read newest-first; and job retention sweeps finished
-- rows by age.
-- +goose Up
CREATE INDEX ix_variants_sku ON product_variants (sku);
CREATE INDEX ix_variants_supplier_item ON product_variants (supplier_item_no)
    WHERE supplier_item_no IS NOT NULL;
CREATE INDEX ix_products_updated ON products (updated_at DESC, id DESC);
CREATE INDEX ix_audit_created ON audit_log (created_at DESC, id DESC);
CREATE INDEX ix_jobs_finished ON jobs (updated_at) WHERE state IN ('done', 'failed', 'dead');

-- +goose Down
DROP INDEX IF EXISTS ix_jobs_finished;
DROP INDEX IF EXISTS ix_audit_created;
DROP INDEX IF EXISTS ix_products_updated;
DROP INDEX IF EXISTS ix_variants_supplier_item;
DROP INDEX IF EXISTS ix_variants_sku;
