-- name: CustomerRosterForSync :many
-- The roster the Signals database needs to compute traits: every customer in a channel, with the
-- lifecycle override resolved here where the segments live rather than shipped as two syncs
-- The override is the SHORTEST-window active segment the customer belongs to, the same rule the
-- recompute applied inline when both sides shared a database
SELECT c.id AS customer_id, c.channel_id, o.new_days, o.active_days, o.at_risk_days
FROM customers c
LEFT JOIN LATERAL (
    SELECT s.lifecycle_new_days AS new_days, s.lifecycle_active_days AS active_days, s.lifecycle_at_risk_days AS at_risk_days
    FROM customer_segment_memberships m
    JOIN customer_segments s ON s.id = m.segment_id
    WHERE m.customer_id = c.id AND s.is_active AND s.lifecycle_at_risk_days IS NOT NULL
    ORDER BY s.lifecycle_at_risk_days ASC, s.id
    LIMIT 1
) o ON TRUE
ORDER BY c.id
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);
