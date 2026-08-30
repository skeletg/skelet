-- name: CreateWebhook :one
INSERT INTO webhooks (
    name, target_url, signing_secret, event_types, kind, channel_id,
    failure_policy, timeout_ms, custom_headers, payload_version
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING *;

-- name: GetWebhookByID :one
SELECT * FROM webhooks WHERE id = $1;

-- name: ListWebhooks :many
SELECT * FROM webhooks ORDER BY created_at DESC;

-- name: ListActiveWebhooksByEvent :many
-- Returns webhooks subscribed to the event, filtered to either global
-- subscribers (channel_id IS NULL) or those scoped to the specified
-- store - the dispatcher passes the event's emitting store; nullable
-- channel_id arg on the query lets the caller include all stores
-- when needed (e.g. CLI introspection)
-- Subscription entries are exact types, "*" (everything of the webhook's
-- kind) or "<prefix>.*" wildcards - mirror webhook.MatchesEntry when
-- changing the matching here
SELECT * FROM webhooks
WHERE is_active = TRUE
  AND EXISTS (
      SELECT 1 FROM unnest(event_types) AS et
      WHERE et = sqlc.arg(event_type)::text
         OR et = '*'
         OR (et LIKE '%.*' AND starts_with(sqlc.arg(event_type)::text, left(et, -1)))
  )
  AND kind = sqlc.arg(kind)::text
  AND (channel_id IS NULL OR channel_id = sqlc.narg(channel_id)::uuid);

-- name: UpdateWebhook :one
UPDATE webhooks
SET name = $2, target_url = $3, event_types = $4, failure_policy = $5,
    timeout_ms = $6, custom_headers = $7, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: SetWebhookActive :one
UPDATE webhooks SET is_active = $2, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: UpdateWebhookSigningSecret :one
UPDATE webhooks SET signing_secret = $2, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteWebhook :exec
DELETE FROM webhooks WHERE id = $1;

-- name: GetWebhookSigningSecret :one
SELECT signing_secret FROM webhooks WHERE id = $1;

-- name: CreateWebhookDelivery :one
INSERT INTO webhook_deliveries (
    webhook_id, event_type, url, request_body, request_headers,
    response_status, response_body, response_headers, duration_ms,
    success, error_message, attempt
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
RETURNING *;

-- name: ListWebhookDeliveries :many
SELECT * FROM webhook_deliveries
WHERE webhook_id = $1
ORDER BY created_at DESC
LIMIT $2;

-- name: WebhookDeliveryStats :one
-- Aggregate delivery health for a single webhook over its stored delivery
-- rows - retries are stored as separate rows, so total/succeeded/failed are
-- attempt-level counts. avg_success_ms averages only successful attempts
-- (failed/timed-out attempts would skew a "typical response time") - the
-- three timestamps are NULL when no matching row exists
SELECT
    COUNT(*)                                                     AS total,
    COUNT(*) FILTER (WHERE success)                              AS succeeded,
    COUNT(*) FILTER (WHERE NOT success)                          AS failed,
    COALESCE(AVG(duration_ms) FILTER (WHERE success), 0)::float8 AS avg_success_ms,
    MAX(created_at)                                              AS last_delivery_at,
    MAX(created_at) FILTER (WHERE success)                       AS last_success_at,
    MAX(created_at) FILTER (WHERE NOT success)                   AS last_failure_at
FROM webhook_deliveries
WHERE webhook_id = $1;

-- name: DeleteWebhookDeliveriesByWebhook :execrows
DELETE FROM webhook_deliveries WHERE webhook_id = $1;

-- name: PruneWebhookDeliveries :execrows
-- Retention: the delivery log grows with every attempt forever otherwise
-- (rides the daily webhook-deliveries-prune job, window from config)
DELETE FROM webhook_deliveries WHERE created_at < $1;

-- name: GetWebhookDelivery :one
SELECT * FROM webhook_deliveries WHERE id = $1;

-- name: EnqueueWebhookDelivery :one
INSERT INTO webhook_queue (webhook_id, event_type, payload)
VALUES ($1, $2, $3)
RETURNING *;

-- name: ClaimDueWebhookQueue :many
-- Claim under a short lease by pushing next_attempt_at forward - a crash
-- mid-delivery makes the row due again once the lease expires, so delivery
-- is at-least-once - SKIP LOCKED partitions work across replicas
UPDATE webhook_queue
SET attempts = attempts + 1,
    next_attempt_at = NOW() + make_interval(secs => sqlc.arg(lease_seconds)::int),
    updated_at = NOW()
WHERE id IN (
    SELECT id FROM webhook_queue
    WHERE status = 'pending' AND next_attempt_at <= NOW()
    ORDER BY next_attempt_at
    LIMIT sqlc.arg(batch_size)::int
    FOR UPDATE SKIP LOCKED
)
RETURNING *;

-- name: DeleteWebhookQueueItem :exec
DELETE FROM webhook_queue WHERE id = $1;

-- name: RescheduleWebhookQueueItem :exec
UPDATE webhook_queue
SET next_attempt_at = $2, last_error = $3, updated_at = NOW()
WHERE id = $1;

-- name: MarkWebhookQueueItemDead :exec
UPDATE webhook_queue
SET status = 'dead', last_error = $2, updated_at = NOW()
WHERE id = $1;

-- name: ListDeadWebhookQueueItems :many
SELECT * FROM webhook_queue
WHERE webhook_id = $1 AND status = 'dead'
ORDER BY updated_at DESC
LIMIT $2;

-- name: RetryWebhookQueueItem :one
-- Reset a dead letter for a fresh delivery cycle
UPDATE webhook_queue
SET status = 'pending', attempts = 0, next_attempt_at = NOW(),
    last_error = NULL, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: ClearDeadWebhookQueueItems :execrows
DELETE FROM webhook_queue WHERE webhook_id = $1 AND status = 'dead';
