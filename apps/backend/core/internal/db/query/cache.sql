-- name: InsertCacheRevalidation :exec
INSERT INTO cache_revalidations (surface, tags, paths, reason, actor_id)
VALUES ($1, $2, $3, $4, $5);

-- name: ListCacheRevalidations :many
SELECT * FROM cache_revalidations ORDER BY created_at DESC LIMIT $1;

-- ==================== Rebuild queue ====================

-- name: EnqueueRebuildTask :execrows
-- Returns rows affected (0 when coalesced against an existing pending task)
INSERT INTO cache_rebuild_tasks (surface, path, tags, reason, dedup_key)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (dedup_key) WHERE status = 'pending' DO NOTHING;

-- name: ClaimDueRebuildTasks :many
-- Lease-claim a batch of due tasks (pending, or running past their lease - a crashed worker's rows) - SKIP LOCKED keeps replicas/overlapping ticks from double-claiming - bumps attempts + pushes the lease out
UPDATE cache_rebuild_tasks
SET status = 'running',
    attempts = attempts + 1,
    next_attempt_at = NOW() + make_interval(secs => sqlc.arg(lease_seconds)::int),
    updated_at = NOW()
WHERE id IN (
    SELECT id FROM cache_rebuild_tasks
    WHERE status IN ('pending', 'running') AND next_attempt_at <= NOW()
    ORDER BY created_at
    LIMIT sqlc.arg(batch_size)
    FOR UPDATE SKIP LOCKED
)
RETURNING *;

-- name: MarkRebuildTaskDone :exec
UPDATE cache_rebuild_tasks
SET status = 'done', done_at = NOW(), last_error = '', updated_at = NOW()
WHERE id = $1;

-- name: FailRebuildTask :exec
-- Caller passes status='pending' to retry with backoff, or 'failed' to dead-letter
UPDATE cache_rebuild_tasks
SET status = sqlc.arg(status),
    last_error = sqlc.arg(last_error),
    next_attempt_at = CASE WHEN sqlc.arg(status) = 'pending'
        THEN NOW() + make_interval(secs => sqlc.arg(backoff_seconds)::int)
        ELSE next_attempt_at END,
    updated_at = NOW()
WHERE id = sqlc.arg(id);

-- name: RebuildTaskStatusCounts :many
SELECT status, COUNT(*)::int AS count FROM cache_rebuild_tasks GROUP BY status;

-- name: ListRecentRebuildTasks :many
SELECT * FROM cache_rebuild_tasks ORDER BY updated_at DESC LIMIT $1;

-- name: DeleteFinishedRebuildTasksBefore :execrows
DELETE FROM cache_rebuild_tasks
WHERE status IN ('done', 'failed') AND updated_at < $1::timestamptz;
