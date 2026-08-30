-- name: InsertCustomerPaymentMethod :one
INSERT INTO customer_payment_methods (customer_id, provider_slug, token, brand, last4, exp_month, exp_year)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (customer_id, provider_slug, token) DO UPDATE
SET brand = EXCLUDED.brand, last4 = EXCLUDED.last4,
    exp_month = EXCLUDED.exp_month, exp_year = EXCLUDED.exp_year
RETURNING *;

-- name: ListCustomerPaymentMethods :many
SELECT * FROM customer_payment_methods
WHERE  customer_id = $1
ORDER  BY created_at DESC;

-- name: GetCustomerPaymentMethod :one
SELECT * FROM customer_payment_methods
WHERE  id = $1 AND customer_id = $2;

-- name: DeleteCustomerPaymentMethod :exec
DELETE FROM customer_payment_methods
WHERE  id = $1 AND customer_id = $2;
