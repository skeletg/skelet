-- Customer wishlist rows

-- name: AddWishlistItem :exec
INSERT INTO wishlist_items (customer_id, product_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: RemoveWishlistItem :exec
DELETE FROM wishlist_items WHERE customer_id = $1 AND product_id = $2;

-- name: ListWishlistProductIDs :many
SELECT product_id FROM wishlist_items
WHERE customer_id = $1
ORDER BY created_at DESC;

-- name: UpsertWishlistShare :one
INSERT INTO wishlist_shares (customer_id, token)
VALUES ($1, $2)
ON CONFLICT (customer_id) DO UPDATE SET customer_id = EXCLUDED.customer_id
RETURNING *;

-- name: GetWishlistShareByCustomer :one
SELECT * FROM wishlist_shares WHERE customer_id = $1;

-- name: GetWishlistShareByToken :one
SELECT * FROM wishlist_shares WHERE token = $1;
