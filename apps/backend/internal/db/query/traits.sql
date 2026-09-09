-- name: CustomerRosterForSync :many
-- The roster the Signals database needs to produce a trait row per customer - identity only, since the lifecycle override is resolved there now that segments live alongside it
SELECT c.id AS customer_id, c.channel_id
FROM customers c
ORDER BY c.id
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);
