-- name: ListEnabledTrackingDestinations :many
SELECT * FROM tracking_destinations
WHERE  channel_id = $1 AND enabled
ORDER  BY created_at;

-- name: GetTrackingDestination :one
SELECT * FROM tracking_destinations WHERE id = $1;

-- name: ListTrackingDestinations :many
SELECT * FROM tracking_destinations
WHERE  channel_id = $1
ORDER  BY created_at;

-- name: CreateTrackingDestination :one
INSERT INTO tracking_destinations (channel_id, kind, name, enabled, events, config, secret)
VALUES (sqlc.arg(channel_id), sqlc.arg(kind), sqlc.arg(name), sqlc.arg(enabled),
        sqlc.arg(events)::text[], sqlc.arg(config), sqlc.arg(secret))
RETURNING *;

-- name: UpdateTrackingDestination :one
-- An empty secret keeps the stored one, so a form that never shows it back can't wipe it
UPDATE tracking_destinations
SET    name = sqlc.arg(name), enabled = sqlc.arg(enabled),
       events = sqlc.arg(events)::text[], config = sqlc.arg(config),
       secret = CASE WHEN sqlc.arg(secret)::text = '' THEN secret ELSE sqlc.arg(secret)::text END,
       updated_at = NOW()
WHERE  id = sqlc.arg(id)
RETURNING *;

-- name: DeleteTrackingDestination :exec
DELETE FROM tracking_destinations WHERE id = $1;

-- name: EnqueueTrackingDelivery :exec
-- A replayed event finds its row already there and changes nothing
INSERT INTO tracking_deliveries (destination_id, event_id, event_name, payload)
VALUES (sqlc.arg(destination_id), sqlc.arg(event_id), sqlc.arg(event_name), sqlc.arg(payload))
ON CONFLICT (destination_id, event_id) DO NOTHING;

-- name: ClaimDueTrackingDeliveries :many
-- Claimed under a short lease by pushing next_attempt_at forward, so a crash mid-send replays the row once it lapses -
-- delivery is at least once, and SKIP LOCKED splits the work across replicas
UPDATE tracking_deliveries
SET    attempts = attempts + 1,
       next_attempt_at = NOW() + make_interval(secs => sqlc.arg(lease_seconds)::int),
       updated_at = NOW()
WHERE  id IN (
    SELECT id FROM tracking_deliveries
    WHERE  status = 'pending' AND next_attempt_at <= NOW()
    ORDER  BY next_attempt_at
    LIMIT  sqlc.arg(batch_size)::int
    FOR UPDATE SKIP LOCKED
)
RETURNING *;

-- name: MarkTrackingDeliverySent :exec
UPDATE tracking_deliveries SET status = 'sent', last_error = '', updated_at = NOW() WHERE id = $1;

-- name: RescheduleTrackingDelivery :exec
UPDATE tracking_deliveries SET next_attempt_at = $2, last_error = $3, updated_at = NOW() WHERE id = $1;

-- name: MarkTrackingDeliveryDead :exec
UPDATE tracking_deliveries SET status = 'dead', last_error = $2, updated_at = NOW() WHERE id = $1;

-- name: RetryTrackingDelivery :exec
-- A dead letter back for a fresh round of attempts
UPDATE tracking_deliveries
SET    status = 'pending', attempts = 0, next_attempt_at = NOW(), last_error = '', updated_at = NOW()
WHERE  id = $1 AND status = 'dead';

-- name: ListTrackingDeliveries :many
SELECT * FROM tracking_deliveries
WHERE  destination_id = sqlc.arg(destination_id)
ORDER  BY created_at DESC
LIMIT  sqlc.arg(lim);

-- name: PruneTrackingDeliveries :execrows
-- The log is for following up a recent problem; a pending row is never pruned
DELETE FROM tracking_deliveries WHERE status <> 'pending' AND updated_at < $1;
