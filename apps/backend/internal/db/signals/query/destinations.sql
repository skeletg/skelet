-- Event destinations and the spine read feeding their forwarding sweep - both live here, so the sweep pages the spine and advances a cursor without crossing a database

-- name: CreateEventDestination :one
INSERT INTO event_destinations (channel_id, name, kind, target_url, secret, event_sources, event_names, is_active)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- The secret is write-only (never returned by the API), so an edit
-- can't round-trip the current value - blank means keep the stored one
-- name: UpdateEventDestination :one
UPDATE event_destinations
SET name = sqlc.arg(name), kind = sqlc.arg(kind), target_url = sqlc.arg(target_url),
    secret = COALESCE(NULLIF(sqlc.arg(secret)::text, ''), secret),
    event_sources = sqlc.arg(event_sources), event_names = sqlc.arg(event_names),
    is_active = sqlc.arg(is_active), updated_at = NOW()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: DeleteEventDestination :exec
DELETE FROM event_destinations WHERE id = $1;

-- name: GetEventDestination :one
SELECT * FROM event_destinations WHERE id = $1;

-- name: ListEventDestinationsByChannel :many
SELECT * FROM event_destinations WHERE channel_id = $1 ORDER BY created_at;

-- name: ListActiveEventDestinations :many
SELECT * FROM event_destinations WHERE is_active = TRUE ORDER BY created_at;

-- Optimistic: only advances past the cursor this sweep read, so concurrent
-- sweeps (cmd/server + cmd/signals both forwarding) can't move it backwards -
-- the losing sweep sees 0 rows and concedes
-- name: AdvanceEventDestinationCursor :execrows
UPDATE event_destinations
SET cursor_created_at = sqlc.arg(cursor_created_at), cursor_event_id = sqlc.arg(cursor_event_id),
    last_error = '', last_forwarded_at = NOW(), updated_at = NOW()
WHERE id = sqlc.arg(id)
  AND cursor_created_at IS NOT DISTINCT FROM sqlc.narg(prev_created_at)
  AND cursor_event_id IS NOT DISTINCT FROM sqlc.narg(prev_event_id);

-- name: SetEventDestinationError :exec
UPDATE event_destinations
SET last_error = $2, updated_at = NOW()
WHERE id = $1;

-- ListEventsForForwarding pages the spine in ingest order after the destination's (created_at, id) cursor - NULL cursor
-- starts from the destination's creation moment - a new destination forwards new events, not the whole history

-- The 5s watermark keeps the page behind any still-committing insert: a row can
-- commit AFTER the sweep read past its created_at, and once the cursor moves it
-- would never be forwarded. Nothing lands that late, so no event is skipped.
-- name: ListEventsForForwarding :many
SELECT id, event_name, occurred_at, channel_id, customer_id, anon_id, source, properties, created_at
FROM analytics_events
WHERE channel_id = sqlc.arg(channel_id)
  AND (created_at, id) > (sqlc.arg(cursor_created_at)::timestamptz, sqlc.arg(cursor_event_id)::uuid)
  AND created_at < NOW() - INTERVAL '5 seconds'
ORDER BY created_at, id
LIMIT sqlc.arg(batch_limit);
