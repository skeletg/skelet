-- name: EnqueuePluginCustomerErasure :exec
-- Once per customer and data set - a redelivered event adds nothing
INSERT INTO plugin_customer_erasures (customer_id, channel_id, email, plugin_slug, data_key)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (customer_id, plugin_slug, data_key) DO NOTHING;

-- name: ListDuePluginCustomerErasures :many
SELECT * FROM plugin_customer_erasures
WHERE next_attempt_at <= sqlc.arg(now)::timestamptz
ORDER BY next_attempt_at
LIMIT sqlc.arg(lim);

-- name: ListPluginCustomerErasuresForCustomer :many
SELECT * FROM plugin_customer_erasures WHERE customer_id = $1 ORDER BY plugin_slug, data_key;

-- name: DeletePluginCustomerErasure :exec
DELETE FROM plugin_customer_erasures WHERE id = $1;

-- name: FailPluginCustomerErasure :exec
UPDATE plugin_customer_erasures
SET attempts = attempts + 1, last_error = sqlc.arg(last_error), next_attempt_at = sqlc.arg(next_attempt_at)
WHERE id = sqlc.arg(id);

-- name: PendingPluginCustomerErasures :one
-- How many are still owed, and since when the oldest
SELECT count(*)::bigint AS pending, coalesce(min(created_at), now())::timestamptz AS oldest
FROM plugin_customer_erasures;
