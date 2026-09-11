-- name: CreateShippingLabel :one
INSERT INTO shipping_labels (fulfillment_id, carrier_slug, format, tracking_number, doc)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetShippingLabel :one
SELECT * FROM shipping_labels WHERE id = $1;

-- name: ListShippingLabelsByFulfillmentIDs :many
SELECT * FROM shipping_labels
WHERE fulfillment_id = ANY($1::uuid[])
ORDER BY created_at ASC;
