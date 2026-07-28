-- The durable job queue. The claim query uses FOR UPDATE SKIP LOCKED so multiple
-- workers can pull disjoint jobs concurrently without blocking each other.

-- name: EnqueueJob :one
INSERT INTO jobs (id, kind, payload, max_attempts, run_after)
VALUES ($1, $2, $3, $4, coalesce($5, now()))
RETURNING id, kind, payload, state, attempts, max_attempts, run_after, locked_by, locked_at, last_error, created_at, updated_at;

-- name: ClaimJob :one
-- Atomically claim the oldest runnable job. SKIP LOCKED lets concurrent workers
-- avoid each other; the increment of attempts happens at claim time so a crash
-- mid-job still counts as an attempt.
UPDATE jobs
SET state = 'running', locked_by = $1, locked_at = now(), attempts = attempts + 1, updated_at = now()
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
    last_error = $2,
    updated_at = now()
WHERE id = $1;

-- name: ReapStuckJobs :execrows
-- Return jobs stuck in 'running' for more than 10 minutes (a crashed worker)
-- back to 'queued' so another worker retries them.
UPDATE jobs
SET state = 'queued', locked_by = NULL, locked_at = NULL, updated_at = now()
WHERE state = 'running' AND locked_at < now() - interval '10 minutes';

-- name: CoalesceQueuedJobsByKind :execrows
-- Collapse duplicate queued jobs of a kind (e.g. many reindex_search enqueues)
-- into the single oldest one, deleting the rest.
DELETE FROM jobs j
WHERE j.kind = $1
  AND j.state = 'queued'
  AND j.id <> (
      SELECT j2.id FROM jobs j2
      WHERE j2.kind = $1 AND j2.state = 'queued'
      ORDER BY j2.run_after
      LIMIT 1
  );
