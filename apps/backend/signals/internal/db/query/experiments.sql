-- Prism's experiment definitions - exposures and goals are scored on the spine

-- name: CreateExperiment :one
INSERT INTO experiments (channel_id, key, name, status, coverage, goal_event, variations, segment_slug)
VALUES (sqlc.arg(channel_id), sqlc.arg(key), sqlc.arg(name), sqlc.arg(status), sqlc.arg(coverage), sqlc.arg(goal_event), sqlc.arg(variations), sqlc.arg(segment_slug))
RETURNING *;

-- name: UpdateExperiment :execrows
UPDATE experiments
SET name = sqlc.arg(name), status = sqlc.arg(status), coverage = sqlc.arg(coverage),
    goal_event = sqlc.arg(goal_event), variations = sqlc.arg(variations), segment_slug = sqlc.arg(segment_slug), updated_at = NOW()
WHERE id = sqlc.arg(id);

-- name: GetExperiment :one
SELECT * FROM experiments WHERE id = sqlc.arg(id);

-- name: ListExperiments :many
SELECT * FROM experiments WHERE channel_id = sqlc.arg(channel_id) ORDER BY created_at DESC;

-- name: ListRunningExperiments :many
SELECT * FROM experiments WHERE channel_id = sqlc.arg(channel_id) AND status = 'running' ORDER BY key;

-- name: DeleteExperiment :exec
DELETE FROM experiments WHERE id = sqlc.arg(id);
