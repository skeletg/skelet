-- Scheduled jobs registry - the engine claims due rows atomically; the admin
-- edits schedule/enabled and creates/deletes webhook jobs

-- name: RegisterScheduledJob :exec
-- Seed a code job's row on boot - the conflict path preserves an operator's schedule/enabled edits (code only sets the default) with ONE exception: a parked row (next_run_at NULL - its handler was missing on a previous boot, e.g. a feature switch or removed plugin) is revived now that the handler is registered again - the revived first run fires per the default cadence; subsequent runs follow the row's (possibly operator-edited) schedule. :exec because the no-op conflict path returns no row
INSERT INTO scheduled_jobs (name, kind, schedule, description, next_run_at, timeout_seconds, enabled)
VALUES ($1, 'code', $2, $3, $4, $5, $6)
ON CONFLICT (name) DO UPDATE
SET next_run_at = EXCLUDED.next_run_at
WHERE scheduled_jobs.next_run_at IS NULL AND scheduled_jobs.kind = 'code';

-- name: CreateWebhookJob :one
INSERT INTO scheduled_jobs (
    name, kind, schedule, description, enabled, payload, next_run_at,
    timezone, max_attempts, retry_backoff_seconds, alert_after_failures,
    created_by, updated_by
)
VALUES ($1, 'webhook', $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $11)
RETURNING *;

-- name: ListScheduledJobs :many
SELECT * FROM scheduled_jobs ORDER BY name;

-- name: GetScheduledJob :one
SELECT * FROM scheduled_jobs WHERE id = $1;

-- name: UpdateScheduledJobSchedule :one
-- Admin edit of cadence / enabled / timezone / retry / alert config - these are configuration, not run state - they don't touch current_attempt / consecutive_failures / alerted (owned by the engine) - caller recomputes next_run_at from the new schedule (nil leaves it unchanged)
UPDATE scheduled_jobs
SET schedule              = $2,
    enabled               = $3,
    timezone              = $4,
    max_attempts          = COALESCE(sqlc.narg('max_attempts'), max_attempts),
    retry_backoff_seconds = COALESCE(sqlc.narg('retry_backoff_seconds'), retry_backoff_seconds),
    alert_after_failures  = COALESCE(sqlc.narg('alert_after_failures'), alert_after_failures),
    timeout_seconds       = COALESCE(sqlc.narg('timeout_seconds'), timeout_seconds),
    next_run_at           = COALESCE(sqlc.narg('next_run_at'), next_run_at),
    updated_by            = sqlc.narg('updated_by'),
    updated_at            = now()
WHERE id = $1
RETURNING *;

-- name: UpdateWebhookJobPayload :one
UPDATE scheduled_jobs
SET payload    = $2,
    description = $3,
    updated_by = sqlc.narg('updated_by'),
    updated_at = now()
WHERE id = $1 AND kind = 'webhook'
RETURNING *;

-- name: SetScheduledJobNextRun :exec
-- "Run now": pull the next run forward (the engine picks it up on its next tick)
UPDATE scheduled_jobs SET next_run_at = $2, updated_at = now() WHERE id = $1;

-- name: DeleteScheduledJob :exec
-- Only webhook (admin-created) jobs are deletable; code jobs are owned by code
DELETE FROM scheduled_jobs WHERE id = $1 AND kind = 'webhook';

-- name: ClaimDueScheduledJobs :many
-- Atomically lease up to batch_size due+enabled jobs - SKIP LOCKED means concurrent replicas never claim the same row, and the lease is the job's own timeout plus a 120s grace so a long run isn't re-claimed mid-flight (RecordScheduledJobResult clears it early)
UPDATE scheduled_jobs
SET locked_until = sqlc.arg('now')::timestamptz + make_interval(secs => scheduled_jobs.timeout_seconds + 120)
WHERE id IN (
    SELECT sj.id FROM scheduled_jobs sj
    WHERE sj.enabled
      AND sj.next_run_at IS NOT NULL
      AND sj.next_run_at <= sqlc.arg('now')
      AND (sj.locked_until IS NULL OR sj.locked_until <= sqlc.arg('now'))
    ORDER BY sj.next_run_at
    LIMIT sqlc.arg('batch_size')
    FOR UPDATE SKIP LOCKED
)
RETURNING *;

-- name: RecordScheduledJobResult :exec
-- Record a run's outcome + retry/alert state, schedule the next run, release the lease - safe to read-modify-write the counters in Go: the claim leases the row to one replica, so there's no competing writer during the run
UPDATE scheduled_jobs
SET last_run_at          = sqlc.arg('ran_at'),
    last_status          = $2,
    last_error           = $3,
    last_duration_ms     = $4,
    next_run_at          = sqlc.narg('next_run_at'),
    current_attempt      = sqlc.arg('current_attempt'),
    consecutive_failures = sqlc.arg('consecutive_failures'),
    alerted              = sqlc.arg('alerted'),
    locked_until         = NULL,
    updated_at           = now()
WHERE id = $1;

-- name: InsertScheduledJobRun :exec
INSERT INTO scheduled_job_runs
    (job_id, job_name, started_at, finished_at, status, error, duration_ms, attempt)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: ListScheduledJobRuns :many
SELECT * FROM scheduled_job_runs WHERE job_id = $1 ORDER BY started_at DESC LIMIT $2;

-- name: PruneScheduledJobRuns :execrows
DELETE FROM scheduled_job_runs WHERE started_at < $1;
