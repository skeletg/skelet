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

-- name: CreateFlowRun :one
INSERT INTO flow_runs (flow_id, customer_id, email, current_node_id, status, resume_at, context)
VALUES (sqlc.arg(flow_id), sqlc.narg(customer_id), sqlc.arg(email), sqlc.arg(current_node_id), sqlc.arg(status), sqlc.narg(resume_at), sqlc.arg(context))
RETURNING *;

-- name: ClaimRunnableFlowRuns :many
-- Runs the engine can advance now - the flow join is load-bearing: a run it would refuse still fills a
-- LIMIT slot, and oldest-first means enough of them starve every newer run behind them
-- OF r so the claim never locks a flow an operator is editing
SELECT r.* FROM flow_runs r
JOIN flows f ON f.id = r.flow_id
WHERE (r.status = 'active' OR (r.status = 'waiting' AND r.resume_at <= NOW()))
  AND f.status = 'active'
  AND f.deleted_at IS NULL
  AND (sqlc.arg('marketing_enabled')::boolean OR f.trigger_config->'transactional' = 'true'::jsonb)
ORDER BY r.created_at
LIMIT sqlc.arg('lim')
FOR UPDATE OF r SKIP LOCKED;

-- name: UpdateFlowRun :exec
UPDATE flow_runs
SET current_node_id = sqlc.arg(current_node_id), status = sqlc.arg(status),
    resume_at = sqlc.narg(resume_at), context = sqlc.arg(context), error = sqlc.arg(error), updated_at = NOW()
WHERE id = sqlc.arg(id);

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
