-- name: CreateDataJob :one
INSERT INTO data_jobs (
    direction, resource, format, status, options, dry_run,
    channel_id, admin_id, filename, object_key, expires_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING *;

-- name: GetDataJob :one
SELECT * FROM data_jobs WHERE id = $1;

-- name: ListDataJobs :many
SELECT * FROM data_jobs
WHERE (sqlc.arg(all_resources)::boolean OR resource = ANY(sqlc.arg(resources)::text[]))
  AND (sqlc.arg(all_directions)::boolean OR direction = sqlc.arg(direction))
  AND (sqlc.arg(all_statuses)::boolean OR status = ANY(sqlc.arg(statuses)::text[]))
  AND (NOT sqlc.arg(scoped)::boolean OR resource = ANY(sqlc.arg(any_channel_resources)::text[])
       OR resource || '|' || channel_id::text = ANY(sqlc.arg(resource_channels)::text[]))
ORDER BY created_at DESC
LIMIT sqlc.arg(page_size) OFFSET sqlc.arg(page_offset);

-- name: CountDataJobs :one
SELECT COUNT(*)::int FROM data_jobs
WHERE (sqlc.arg(all_resources)::boolean OR resource = ANY(sqlc.arg(resources)::text[]))
  AND (sqlc.arg(all_directions)::boolean OR direction = sqlc.arg(direction))
  AND (sqlc.arg(all_statuses)::boolean OR status = ANY(sqlc.arg(statuses)::text[]))
  AND (NOT sqlc.arg(scoped)::boolean OR resource = ANY(sqlc.arg(any_channel_resources)::text[])
       OR resource || '|' || channel_id::text = ANY(sqlc.arg(resource_channels)::text[]));

-- name: ClaimDueDataJobs :many
-- Lease-claim queued work, plus running rows whose lease lapsed (a crashed runner's) - SKIP LOCKED keeps
-- replicas off each other's rows, and attempts is what stops a job that panics the process from looping forever
UPDATE data_jobs
SET status = 'running',
    attempts = attempts + 1,
    started_at = COALESCE(started_at, NOW()),
    lease_until = NOW() + make_interval(secs => sqlc.arg(lease_seconds)::int)
WHERE id IN (
    SELECT due.id FROM data_jobs due
    WHERE (due.status = 'queued' OR (due.status = 'running' AND due.lease_until < NOW()))
      AND due.attempts < sqlc.arg(max_attempts)
    ORDER BY due.created_at
    LIMIT sqlc.arg(batch_size)
    FOR UPDATE SKIP LOCKED
)
RETURNING *;

-- name: RenewDataJobLease :execrows
-- Returns rows affected, which is how a running job learns it was cancelled: the WHERE no longer matches once
-- the status flips, so 0 rows IS the stop signal - no extra query, and it works across replicas
UPDATE data_jobs
SET lease_until = NOW() + make_interval(secs => sqlc.arg(lease_seconds)::int),
    processed_rows = sqlc.arg(processed_rows),
    total_rows = GREATEST(total_rows, sqlc.arg(total_rows))
WHERE id = sqlc.arg(id) AND status = 'running';

-- name: FinishDataJob :one
UPDATE data_jobs
SET status = sqlc.arg(status),
    total_rows = sqlc.arg(total_rows),
    processed_rows = sqlc.arg(processed_rows),
    created_rows = sqlc.arg(created_rows),
    updated_rows = sqlc.arg(updated_rows),
    skipped_rows = sqlc.arg(skipped_rows),
    failed_rows = sqlc.arg(failed_rows),
    error_summary = sqlc.arg(error_summary),
    warnings = sqlc.arg(warnings),
    errors_truncated = sqlc.arg(errors_truncated),
    object_key = sqlc.arg(object_key),
    byte_size = sqlc.arg(byte_size),
    finished_at = NOW(),
    lease_until = NULL
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: CancelDataJob :one
-- A running job is stopped by its runner noticing the lease renewal stopped matching, within one renewal interval
UPDATE data_jobs
SET status = 'cancelled', finished_at = NOW(), lease_until = NULL
WHERE id = $1 AND status IN ('queued', 'running')
RETURNING *;

-- name: InsertDataJobErrors :exec
-- One statement for the whole batch - a 1000-error import would otherwise be 1000 round trips
INSERT INTO data_job_errors (job_id, line, ref, message)
SELECT sqlc.arg(job_id)::uuid,
       unnest(sqlc.arg(lines)::int[]),
       unnest(sqlc.arg(refs)::text[]),
       unnest(sqlc.arg(messages)::text[]);

-- name: ListDataJobErrors :many
SELECT * FROM data_job_errors
WHERE job_id = sqlc.arg(job_id)
ORDER BY line
LIMIT sqlc.arg(page_size) OFFSET sqlc.arg(page_offset);

-- name: CountDataJobErrors :one
SELECT COUNT(*)::int FROM data_job_errors WHERE job_id = $1;

-- name: ReapStuckDataJobs :execrows
-- A job whose runner died maxAttempts times is left running with a lapsed lease, and the claim query excludes it
-- forever - without this it shows as running for eternity and the dashboard polls behind it just as long
UPDATE data_jobs
SET status = 'failed',
    error_summary = CASE WHEN error_summary = '' THEN 'the runner stopped responding and the job was not retried again' ELSE error_summary END,
    finished_at = NOW(),
    lease_until = NULL
WHERE status = 'running'
  AND lease_until IS NOT NULL
  AND lease_until < NOW()
  AND attempts >= sqlc.arg(max_attempts);

-- name: ExpireDataJobArtifacts :many
-- Returns the keys to delete from the object store - the row stays so the history survives its artifact
UPDATE data_jobs
SET object_key = '', byte_size = 0
WHERE object_key <> '' AND expires_at IS NOT NULL AND expires_at < NOW()
RETURNING id, object_key;

-- name: DeleteDataJobsBefore :execrows
DELETE FROM data_jobs
WHERE finished_at IS NOT NULL AND finished_at < sqlc.arg(before)::timestamptz;
