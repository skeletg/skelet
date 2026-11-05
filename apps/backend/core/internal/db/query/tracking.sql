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

-- name: EnqueueTrackingEvent :exec
-- The event once and a delivery for each destination, in one statement - so a replayed event, which finds its key
-- already stored, adds nothing at all, even after its deliveries were sent and its data scrubbed
WITH ev AS (
    INSERT INTO tracking_events (channel_id, event_id, event_name, customer_id, anon_id, email, payload)
    VALUES (sqlc.arg(channel_id), sqlc.arg(event_id), sqlc.arg(event_name), sqlc.narg(customer_id),
            sqlc.arg(anon_id), sqlc.arg(email), sqlc.arg(payload))
    ON CONFLICT (channel_id, event_name, event_id) DO NOTHING
    RETURNING id
)
INSERT INTO tracking_deliveries (destination_id, tracking_event_id)
SELECT dest, ev.id FROM ev, unnest(sqlc.arg(destination_ids)::uuid[]) AS dest
ON CONFLICT (destination_id, tracking_event_id) DO NOTHING;

-- name: ClaimDueTrackingDeliveries :many
-- Claimed under a short lease by pushing next_attempt_at forward, so a crash mid-send replays the row once it lapses -
-- delivery is at least once, and SKIP LOCKED splits the work across replicas. Same columns as ListTrackingDeliveries
WITH claimed AS (
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
    RETURNING *
)
SELECT c.id, c.destination_id, e.event_id, e.event_name, e.payload, c.status, c.attempts, c.next_attempt_at,
       c.last_error, c.created_at, c.updated_at
FROM   claimed c
JOIN   tracking_events e ON e.id = c.tracking_event_id;

-- name: DeleteTrackingDelivery :exec
-- A sent delivery is done with - the log keeps what is pending or failed, and the event row guards against a replay.
-- Its destination's last success is stamped at most once a minute, so a busy one isn't rewritten on every send
WITH sent AS (DELETE FROM tracking_deliveries x WHERE x.id = $1 RETURNING x.destination_id)
UPDATE tracking_destinations t
SET    last_sent_at = NOW()
FROM   sent
WHERE  t.id = sent.destination_id
  AND  (t.last_sent_at IS NULL OR t.last_sent_at < NOW() - INTERVAL '1 minute');

-- name: RescheduleTrackingDelivery :exec
UPDATE tracking_deliveries SET next_attempt_at = $2, last_error = $3, updated_at = NOW() WHERE id = $1;

-- name: MarkTrackingDeliveryDead :exec
UPDATE tracking_deliveries SET status = 'dead', last_error = $2, updated_at = NOW() WHERE id = $1;

-- name: RetryTrackingDelivery :execrows
-- A dead letter back for a fresh round of attempts
UPDATE tracking_deliveries
SET    status = 'pending', attempts = 0, next_attempt_at = NOW(), last_error = '', updated_at = NOW()
WHERE  id = $1 AND status = 'dead';

-- name: ListTrackingDeliveries :many
-- Same columns as ClaimDueTrackingDeliveries
SELECT d.id, d.destination_id, e.event_id, e.event_name, e.payload, d.status, d.attempts, d.next_attempt_at,
       d.last_error, d.created_at, d.updated_at
FROM   tracking_deliveries d
JOIN   tracking_events e ON e.id = d.tracking_event_id
WHERE  d.destination_id = sqlc.arg(destination_id)
  AND  (sqlc.arg(status)::text = '' OR d.status = sqlc.arg(status)::text) -- empty lists every row
ORDER  BY d.created_at DESC
LIMIT  sqlc.arg(lim);

-- name: GetTrackingDelivery :one
-- Same columns as ClaimDueTrackingDeliveries
SELECT d.id, d.destination_id, e.event_id, e.event_name, e.payload, d.status, d.attempts, d.next_attempt_at,
       d.last_error, d.created_at, d.updated_at
FROM   tracking_deliveries d
JOIN   tracking_events e ON e.id = d.tracking_event_id
WHERE  d.id = $1;

-- name: ImportTrackingDestination :one
-- Always disabled and without a secret - the admin re-enters it, since Signals never hands one over. A second import of
-- the same Signals row returns nothing
INSERT INTO tracking_destinations (channel_id, kind, name, enabled, events, config, secret, imported_from)
VALUES (sqlc.arg(channel_id), sqlc.arg(kind), sqlc.arg(name), FALSE, '{}'::text[], sqlc.arg(config), '', sqlc.arg(imported_from))
ON CONFLICT (imported_from) WHERE imported_from IS NOT NULL DO NOTHING
RETURNING *;

-- name: TrackingDestinationHealth :many
-- Each destination of a channel with its last success, last failure and failures in the past day
SELECT d.id,
       COALESCE(d.last_sent_at, 'epoch'::timestamptz)::timestamptz AS last_sent_at, -- the epoch stands for never
       COALESCE((SELECT x.last_error FROM tracking_deliveries x WHERE x.destination_id = d.id AND x.status = 'dead'
                 ORDER BY x.created_at DESC LIMIT 1), '')::text AS last_error,
       (SELECT COUNT(*) FROM tracking_deliveries x WHERE x.destination_id = d.id AND x.status = 'dead'
          AND x.created_at > NOW() - INTERVAL '24 hours')::int AS dead_24h
FROM   tracking_destinations d
WHERE  d.channel_id = $1;

-- name: PruneTrackingDeliveries :execrows
-- A dead letter is kept for a retry until it is past following up; a pending row is never pruned
DELETE FROM tracking_deliveries WHERE status = 'dead' AND updated_at < $1;

-- name: ScrubSettledTrackingEvents :execrows
-- An event with nothing left to deliver keeps only its key, for the replay window - the visitor's data goes at once,
-- which also covers the opt-out an erasure sends after it has deleted everything else
UPDATE tracking_events e
SET    payload = '{}'::jsonb, customer_id = NULL, anon_id = '', email = ''
WHERE  e.payload <> '{}'::jsonb
  AND  NOT EXISTS (SELECT 1 FROM tracking_deliveries d WHERE d.tracking_event_id = e.id);

-- name: PruneTrackingEvents :execrows
-- Past the replay window an event with nothing left to deliver goes too
DELETE FROM tracking_events e
WHERE  e.created_at < $1
  AND  NOT EXISTS (SELECT 1 FROM tracking_deliveries d WHERE d.tracking_event_id = e.id);
