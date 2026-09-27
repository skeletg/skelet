-- name: CreateFlow :one
INSERT INTO flows (channel_id, name, trigger_type, trigger_config, graph)
VALUES (sqlc.arg(channel_id), sqlc.arg(name), sqlc.arg(trigger_type), sqlc.arg(trigger_config), sqlc.arg(graph))
RETURNING *;

-- name: GetFlow :one
SELECT * FROM flows WHERE id = sqlc.arg(id);

-- name: ListFlows :many
-- Live flows for the dashboard list - archived rows are fetched separately
SELECT * FROM flows
WHERE channel_id = sqlc.arg(channel_id)
  AND (sqlc.arg(include_deleted)::bool OR deleted_at IS NULL)
ORDER BY created_at DESC;

-- name: ListActiveFlowsByTrigger :many
-- Enrollment path - an archived flow must never enroll anyone again, so the
-- archive filter belongs here and not only in the read APIs
SELECT * FROM flows
WHERE channel_id = sqlc.arg(channel_id) AND trigger_type = sqlc.arg(trigger_type)
  AND status = 'active' AND deleted_at IS NULL;

-- name: UpdateFlow :execrows
UPDATE flows
SET name = sqlc.arg(name), trigger_type = sqlc.arg(trigger_type),
    trigger_config = sqlc.arg(trigger_config), graph = sqlc.arg(graph), updated_at = NOW()
WHERE id = sqlc.arg(id);

-- name: SetFlowStatus :execrows
UPDATE flows SET status = sqlc.arg(status), updated_at = NOW() WHERE id = sqlc.arg(id);

-- name: SoftDeleteFlow :execrows
-- Soft delete - keeps flow_runs and their history pointing at a real row
UPDATE flows SET deleted_at = NOW(), updated_at = NOW()
WHERE id = sqlc.arg(id) AND deleted_at IS NULL;

-- name: RestoreFlow :execrows
UPDATE flows SET deleted_at = NULL, updated_at = NOW()
WHERE id = sqlc.arg(id) AND deleted_at IS NOT NULL;

-- name: CountLiveRunsForCustomer :one
SELECT COUNT(*) FROM flow_runs
WHERE flow_id = sqlc.arg(flow_id) AND customer_id = sqlc.arg(customer_id) AND status IN ('active', 'waiting');

-- name: CountLiveGuestRuns :one
-- Live-run dedup for guest enrollments (no customer), keyed by email
SELECT COUNT(*) FROM flow_runs
WHERE flow_id = sqlc.arg(flow_id) AND customer_id IS NULL AND email = sqlc.arg(email) AND status IN ('active', 'waiting');

-- name: FlowRunExistsForKey :one
SELECT EXISTS (SELECT 1 FROM flow_runs WHERE flow_id = sqlc.arg(flow_id) AND enrollment_key = sqlc.arg(enrollment_key)) AS held;

-- name: CreateFlowRun :one
-- A keyed moment that already has its run inserts nothing and returns no row
INSERT INTO flow_runs (flow_id, customer_id, email, current_node_id, status, resume_at, context, enrollment_key)
VALUES (sqlc.arg(flow_id), sqlc.narg(customer_id), sqlc.arg(email), sqlc.arg(current_node_id), sqlc.arg(status), sqlc.narg(resume_at), sqlc.arg(context), sqlc.arg(enrollment_key))
ON CONFLICT (flow_id, enrollment_key) WHERE enrollment_key <> '' DO NOTHING
RETURNING *;

-- name: ClaimRunnableFlowRuns :many
-- Leases runnable runs by pushing resume_at past the lease; channels take turns, and the flow join stays, or a refused run would still fill a LIMIT slot
WITH candidates AS (
    SELECT c.id, c.created_at, row_number() OVER (PARTITION BY f.channel_id ORDER BY c.created_at) AS turn
    FROM flows f
    CROSS JOIN LATERAL (
        -- Due order, on flow_runs_flow_due - an active run with no resume_at is due immediately
        SELECT r.id, r.created_at FROM flow_runs r
        WHERE r.flow_id = f.id AND r.status IN ('active', 'waiting')
          AND COALESCE(r.resume_at, CASE WHEN r.status = 'active' THEN '-infinity'::timestamptz END) <= NOW()
        ORDER BY COALESCE(r.resume_at, CASE WHEN r.status = 'active' THEN '-infinity'::timestamptz END)
        LIMIT sqlc.arg('lim')
    ) c
    WHERE f.status = 'active' AND f.deleted_at IS NULL
)
UPDATE flow_runs SET resume_at = NOW() + make_interval(secs => sqlc.arg(lease_seconds)::float8), updated_at = NOW()
WHERE id IN (
    SELECT r.id FROM flow_runs r
    WHERE r.id IN (SELECT id FROM candidates ORDER BY turn, created_at LIMIT sqlc.arg('lim'))
      AND r.status IN ('active', 'waiting')
      AND (r.resume_at <= NOW() OR (r.status = 'active' AND r.resume_at IS NULL))
    FOR UPDATE SKIP LOCKED
)
RETURNING *;

-- name: UpdateFlowRun :exec
UPDATE flow_runs
SET current_node_id = sqlc.arg(current_node_id), status = sqlc.arg(status),
    resume_at = sqlc.narg(resume_at), context = sqlc.arg(context), error = sqlc.arg(error), updated_at = NOW()
WHERE id = sqlc.arg(id);

-- name: PruneFinishedFlowRuns :execrows
-- Finished runs past the send-history window - live runs are never touched
DELETE FROM flow_runs WHERE status IN ('completed', 'cancelled', 'failed') AND updated_at < sqlc.arg(before);

-- name: DeleteFlowRunsForCustomers :execrows
-- Runs for customers the roster just dropped - a live run for a deleted customer would keep trying to mail them
DELETE FROM flow_runs WHERE customer_id = ANY(sqlc.arg(customer_ids)::uuid[]);

-- name: ListFlowRunsByFlow :many
SELECT * FROM flow_runs WHERE flow_id = sqlc.arg(flow_id) ORDER BY created_at DESC LIMIT sqlc.arg('lim');

-- name: ListFlowRunsForCustomer :many
-- Every automation-flow enrollment for a customer (Synapse profile / Pulse
-- view), newest first, joined to the flow for its name + trigger
SELECT r.id, r.flow_id, r.status, r.created_at, r.updated_at,
       f.name AS flow_name, f.trigger_type AS trigger_type
FROM flow_runs r
JOIN flows f ON f.id = r.flow_id
WHERE r.customer_id = sqlc.arg(customer_id)
ORDER BY r.created_at DESC
LIMIT 50;
