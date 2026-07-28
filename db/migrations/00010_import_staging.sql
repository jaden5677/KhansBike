-- 00010_import_staging: the xlsx importer's landing zone. Everything imports into
-- these staging tables first (dry-run by default); a human reviews the proposed
-- rows and their detected issues, then commits. import_batches.status is plain
-- text (dry_run|reviewing|committed|aborted) so states can be added freely.
-- +goose Up
CREATE TYPE import_decision AS ENUM ('pending', 'accept', 'skip', 'merge');

CREATE TABLE import_batches (
    id           uuid PRIMARY KEY,
    filename     text NOT NULL,
    sha256       bytea NOT NULL,                     -- digest of the uploaded workbook
    status       text NOT NULL DEFAULT 'dry_run',
    created_by   uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    committed_at timestamptz
);

CREATE TABLE import_rows (
    id                uuid PRIMARY KEY,
    batch_id          uuid NOT NULL REFERENCES import_batches(id) ON DELETE CASCADE,
    sheet_name        text NOT NULL,
    row_index         int NOT NULL,                  -- 0-based source row within the sheet
    raw               jsonb NOT NULL,                -- verbatim cell values
    proposed          jsonb NOT NULL,                -- normalised product/variant/prices/attributes
    issues            jsonb NOT NULL DEFAULT '[]',   -- detected defects: typos, dupes, missing price, ...
    decision          import_decision NOT NULL DEFAULT 'pending',
    target_product_id uuid REFERENCES products(id) ON DELETE SET NULL
);
CREATE INDEX ix_import_rows_batch ON import_rows (batch_id);
CREATE INDEX ix_import_rows_decision ON import_rows (batch_id, decision);

-- +goose Down
DROP TABLE IF EXISTS import_rows;
DROP TABLE IF EXISTS import_batches;
DROP TYPE IF EXISTS import_decision;
