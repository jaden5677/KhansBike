-- 00001_extensions: enable the Postgres extensions the whole schema depends on
-- and install an IMMUTABLE unaccent wrapper usable in index/generated-column
-- expressions. We rely on Postgres-specific features throughout; portability is
-- a non-goal.
-- +goose Up
CREATE EXTENSION IF NOT EXISTS citext;    -- case-insensitive email column
CREATE EXTENSION IF NOT EXISTS pg_trgm;   -- trigram typo-tolerant search + suggest
CREATE EXTENSION IF NOT EXISTS unaccent;  -- diacritic-insensitive matching
CREATE EXTENSION IF NOT EXISTS btree_gin; -- mix scalar and gin columns in one index
CREATE EXTENSION IF NOT EXISTS ltree;     -- category tree paths
CREATE EXTENSION IF NOT EXISTS pgcrypto;  -- gen_random_uuid() default for trigger-inserted job rows

-- The stock unaccent() is STABLE, not IMMUTABLE, so it cannot be used in index
-- expressions. This thin wrapper pins the dictionary and is safe to mark
-- IMMUTABLE, which the search vector build in 00008 relies on.
-- +goose StatementBegin
CREATE FUNCTION f_unaccent(text) RETURNS text
    LANGUAGE sql IMMUTABLE PARALLEL SAFE STRICT
    AS $$ SELECT unaccent('unaccent', $1) $$;
-- +goose StatementEnd

-- +goose Down
DROP FUNCTION IF EXISTS f_unaccent(text);
-- Extensions are intentionally left installed on Down: they are database-wide,
-- harmless when unused, and dropping them could break unrelated objects.
