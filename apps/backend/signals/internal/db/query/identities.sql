-- The GDPR erasure queue for the spine's identity mapping

-- name: EnqueueIdentityErasure :exec
-- Erasures batch through this queue - one ClickHouse mutation per sweep, not per request - and the upsert makes re-requesting free
INSERT INTO gdpr_identity_erasures (customer_id) VALUES (sqlc.arg(customer_id))
ON CONFLICT (customer_id) DO UPDATE SET requested_at = now();

-- name: QueuedIdentityErasures :many
-- older_than holds back fresh requests until every process's buffered identifies have flushed
SELECT customer_id FROM gdpr_identity_erasures WHERE requested_at < sqlc.arg(older_than) ORDER BY requested_at LIMIT sqlc.arg(lim);

-- name: DeleteIdentityErasures :exec
-- Only after the batched spine delete landed - a failed sweep retries these.
DELETE FROM gdpr_identity_erasures WHERE customer_id = ANY(sqlc.arg(customer_ids)::uuid[]);
