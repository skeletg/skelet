-- Spine read for the event-forwarding sweep (destination.Service). The destination rows + cursor live in commerce
-- (event_destinations); only this page-the-spine read moved here

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
