-- Durable at-most-once markers for sdk.SubscribeAsyncOnce - claim wins exactly once per key,
-- release un-claims after a handler error so the next outbox replay retries

-- name: ClaimPluginEventDedup :execrows
INSERT INTO plugin_event_dedup (plugin_slug, event_type, dedup_key)
VALUES ($1, $2, $3)
ON CONFLICT DO NOTHING;

-- name: ReleasePluginEventDedup :exec
DELETE FROM plugin_event_dedup
WHERE plugin_slug = $1 AND event_type = $2 AND dedup_key = $3;

-- name: PrunePluginEventDedup :execrows
DELETE FROM plugin_event_dedup WHERE processed_at < $1;
