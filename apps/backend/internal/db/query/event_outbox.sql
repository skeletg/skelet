-- ==================== Event outbox (internal/outbox relay) ====================

-- name: InsertOutboxEvent :one
INSERT INTO event_outbox (event_type, payload)
VALUES ($1, $2)
RETURNING *;

-- name: ClaimDueOutboxEvents :many
-- Lease-based relay claim: atomically bump next_attempt_at into the
-- future (the lease) for up to $1 due rows and return them - the claim
-- transaction is just this UPDATE - dispatch happens OUTSIDE any tx so
-- a slow subscriber (webhook retry chain) never holds a DB transaction
-- open - if the process dies mid-dispatch the lease expires and the row
-- is re-claimed: at-least-once - FOR UPDATE SKIP LOCKED keeps replicas
-- and overlapping ticks from double-claiming
UPDATE event_outbox
SET attempts = attempts + 1,
    next_attempt_at = NOW() + make_interval(secs => sqlc.arg(lease_seconds)::int)
WHERE id IN (
    SELECT id FROM event_outbox
    WHERE delivered_at IS NULL AND next_attempt_at <= NOW()
    ORDER BY created_at
    LIMIT sqlc.arg(batch_size)
    FOR UPDATE SKIP LOCKED
)
RETURNING *;

-- name: MarkOutboxDelivered :exec
UPDATE event_outbox
SET delivered_at = NOW()
WHERE id = $1;

-- name: DeleteDeliveredOutboxBefore :execrows
DELETE FROM event_outbox
WHERE delivered_at IS NOT NULL AND delivered_at < $1::timestamptz;
