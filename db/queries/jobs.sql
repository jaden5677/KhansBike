-- The durable job queue. The claim query uses FOR UPDATE SKIP LOCKED so multiple
-- workers can pull disjoint jobs concurrently without blocking each other.

-- name: EnqueueJob :one
INSERT INTO jobs (id, kind, payload, max_attempts, run_after)
VALUES (sqlc.arg(id), sqlc.arg(kind), sqlc.arg(payload), sqlc.arg(max_attempts),
        coalesce(sqlc.narg(run_after)::timestamptz, now()))
RETURNING id, kind, payload, state, attempts, max_attempts, run_after, locked_by, locked_at, last_error, created_at, updated_at;

-- name: ClaimJob :one
-- Atomically claim the oldest runnable job. SKIP LOCKED lets concurrent workers
-- avoid each other; the increment of attempts happens at claim time so a crash
-- mid-job still counts as an attempt.
UPDATE jobs
SET state = 'running', locked_by = sqlc.arg(locked_by)::text, locked_at = now(), attempts = attempts + 1, updated_at = now()
WHERE id = (
    SELECT id FROM jobs
    WHERE state = 'queued' AND run_after <= now()
    ORDER BY run_after
    FOR UPDATE SKIP LOCKED
    LIMIT 1
)
RETURNING id, kind, payload, state, attempts, max_attempts, run_after, locked_by, locked_at, last_error, created_at, updated_at;

-- name: CompleteJob :exec
UPDATE jobs SET state = 'done', locked_by = NULL, locked_at = NULL, updated_at = now()
WHERE id = $1;

-- name: RetryJob :exec
-- Reschedule with exponential backoff and jitter, or dead-letter once attempts
-- reach max_attempts.
UPDATE jobs
SET state = CASE WHEN attempts >= max_attempts THEN 'dead'::job_state ELSE 'queued'::job_state END,
    run_after = now() + (interval '1 second' * pow(2, attempts) * (0.5 + random())),
    locked_by = NULL,
    locked_at = NULL,
    last_error = sqlc.arg(last_error)::text,
    updated_at = now()
WHERE id = sqlc.arg(id);

-- name: FailJob :exec
-- A permanent failure (bad payload, undecodable image): retrying cannot help.
UPDATE jobs
SET state = 'failed', locked_by = NULL, locked_at = NULL, last_error = sqlc.arg(last_error)::text, updated_at = now()
WHERE id = sqlc.arg(id);

-- name: ReapStuckJobs :execrows
-- Jobs stuck in 'running' for more than 10 minutes belong to a crashed or hung
-- worker. Return them to the queue, or dead-letter them once they have used up
-- their attempts, so a job that kills its worker cannot loop forever.
UPDATE jobs
SET state = CASE WHEN attempts >= max_attempts THEN 'dead'::job_state ELSE 'queued'::job_state END,
    locked_by = NULL,
    locked_at = NULL,
    last_error = 'lease expired: worker stopped before finishing',
    updated_at = now()
WHERE state = 'running' AND locked_at < now() - interval '10 minutes';

-- name: DiscardQueuedJobsByKind :execrows
-- Drops every queued job of a kind. The reindex handler calls this before it
-- rebuilds: the rebuild reads the current state, so every change that queued
-- one of those jobs is covered, and changes committed later enqueue anew.
DELETE FROM jobs WHERE kind = $1 AND state = 'queued';

-- name: DeleteFinishedJobs :execrows
-- Retention: successful jobs are kept for a week, failures for a month so they
-- can still be diagnosed.
DELETE FROM jobs
WHERE (state = 'done' AND updated_at < now() - interval '7 days')
   OR (state IN ('failed', 'dead') AND updated_at < now() - interval '30 days');
