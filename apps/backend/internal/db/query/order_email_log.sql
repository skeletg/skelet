-- name: OrderEmailExists :one
SELECT EXISTS (
    SELECT 1 FROM order_email_log
    WHERE order_id = $1 AND kind = $2 AND ref = $3
);

-- name: InsertOrderEmailLog :exec
INSERT INTO order_email_log (order_id, kind, ref)
VALUES ($1, $2, $3)
ON CONFLICT (order_id, kind, ref) DO NOTHING;
