-- 00009_jobs: the durable Postgres-backed job queue, the audit log, and the
-- statement-level triggers that enqueue a search reindex when EAV values or
-- variants change (defined here, after jobs exists, so the trigger can insert
-- into it). jobs.kind is plain text (churns as new job types are added).
-- jobs.id defaults to a v4 uuid so trigger-inserted rows get an id without the
-- app; application inserts still pass a UUIDv7.
-- +goose Up
CREATE TYPE job_state AS ENUM ('queued', 'running', 'done', 'failed', 'dead');

CREATE TABLE jobs (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    kind         text NOT NULL,
    payload      jsonb NOT NULL DEFAULT '{}',
    state        job_state NOT NULL DEFAULT 'queued',
    attempts     int NOT NULL DEFAULT 0,
    max_attempts int NOT NULL DEFAULT 5,
    run_after    timestamptz NOT NULL DEFAULT now(),
    locked_by    text,
    locked_at    timestamptz,
    last_error   text,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);
-- Partial index matching the SKIP LOCKED claim query (state='queued' ORDER BY run_after).
CREATE INDEX ix_jobs_claim ON jobs (run_after) WHERE state = 'queued';
-- Supports the reaper scanning for stuck 'running' rows by locked_at.
CREATE INDEX ix_jobs_reap ON jobs (locked_at) WHERE state = 'running';

CREATE TABLE audit_log (
    id            uuid PRIMARY KEY,
    actor_user_id uuid REFERENCES users(id) ON DELETE SET NULL,
    actor_kind    text NOT NULL,                    -- 'admin' | 'device' | 'system'
    action        text NOT NULL,
    entity_type   text NOT NULL,
    entity_id     uuid,
    before        jsonb,
    after         jsonb,
    ip            inet,
    created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ix_audit_entity ON audit_log (entity_type, entity_id);
CREATE INDEX ix_audit_actor ON audit_log (actor_user_id);

-- Statement-level triggers: EAV/variant changes only ENQUEUE a reindex job
-- rather than doing the (expensive, multi-join) vector rebuild inline. A
-- coalescing worker collapses duplicate jobs.
-- +goose StatementBegin
CREATE FUNCTION trg_enqueue_reindex() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO jobs (kind, payload) VALUES ('reindex_search', '{}'::jsonb);
    RETURN NULL;  -- AFTER STATEMENT trigger; return value is ignored
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER pav_reindex
    AFTER INSERT OR UPDATE OR DELETE ON product_attribute_values
    FOR EACH STATEMENT EXECUTE FUNCTION trg_enqueue_reindex();
CREATE TRIGGER variant_reindex
    AFTER INSERT OR UPDATE OR DELETE ON product_variants
    FOR EACH STATEMENT EXECUTE FUNCTION trg_enqueue_reindex();

-- +goose Down
DROP TRIGGER IF EXISTS variant_reindex ON product_variants;
DROP TRIGGER IF EXISTS pav_reindex ON product_attribute_values;
DROP FUNCTION IF EXISTS trg_enqueue_reindex();
DROP TABLE IF EXISTS audit_log;
DROP TABLE IF EXISTS jobs;
DROP TYPE IF EXISTS job_state;
