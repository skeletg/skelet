-- ==================== Delivery options ====================

-- name: ListDeliveryOptionsByChannel :many
SELECT * FROM delivery_options
WHERE channel_id = $1
ORDER BY sort_order, created_at;

-- name: GetDeliveryOption :one
SELECT * FROM delivery_options WHERE id = $1;

-- name: CreateDeliveryOption :one
INSERT INTO delivery_options (
    channel_id, kind, name, description, enabled, sort_order, block_modes, point_source, carrier_slug
)
VALUES (
    sqlc.arg(channel_id), sqlc.arg(kind), sqlc.arg(name), sqlc.arg(description), sqlc.arg(enabled), sqlc.arg(sort_order),
    sqlc.arg(block_modes), sqlc.arg(point_source), sqlc.arg(carrier_slug)
)
RETURNING *;

-- name: UpdateDeliveryOption :one
UPDATE delivery_options SET
    name                 = sqlc.arg(name),
    description          = sqlc.arg(description),
    enabled              = sqlc.arg(enabled),
    sort_order           = sqlc.arg(sort_order),
    block_modes          = sqlc.arg(block_modes),
    point_source         = sqlc.arg(point_source),
    carrier_slug         = sqlc.arg(carrier_slug),
    updated_at           = NOW()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: DeleteDeliveryOption :exec
DELETE FROM delivery_options WHERE id = $1;

-- ==================== Delivery points ====================

-- name: ListDeliveryPointsByChannel :many
SELECT * FROM delivery_points
WHERE channel_id = $1
ORDER BY sort_order, name;

-- name: SearchDeliveryPoints :many
-- Active points of a kind in the shopper's country - text matches name, city or postcode, and an empty search takes them all. Nearest-first ordering needs coordinates the admin doesn't enter, so a postcode match ranks first instead
SELECT * FROM delivery_points
WHERE channel_id = sqlc.arg(channel_id)
  AND kind = sqlc.arg(kind)
  AND is_active
  AND (sqlc.arg(country_code)::text = '' OR country_code = sqlc.arg(country_code))
  AND (sqlc.arg(search)::text = ''
       OR name ILIKE '%' || sqlc.arg(search) || '%'
       OR city ILIKE '%' || sqlc.arg(search) || '%'
       OR postal_code ILIKE sqlc.arg(search) || '%')
ORDER BY (postal_code = sqlc.arg(postal_code)) DESC, sort_order, name
LIMIT sqlc.arg(row_limit);

-- name: GetDeliveryPoint :one
SELECT * FROM delivery_points WHERE channel_id = $1 AND id = $2 AND is_active;

-- name: CountActiveDeliveryPoints :one
SELECT count(*) FROM delivery_points WHERE channel_id = $1 AND kind = $2 AND is_active;

-- name: UpsertDeliveryPoint :one
INSERT INTO delivery_points (
    id, channel_id, kind, name, street1, street2, city, region, postal_code, country_code, instructions, hours, is_active, sort_order
)
VALUES (
    sqlc.arg(id), sqlc.arg(channel_id), sqlc.arg(kind), sqlc.arg(name), sqlc.arg(street1), sqlc.arg(street2), sqlc.arg(city),
    sqlc.arg(region), sqlc.arg(postal_code), sqlc.arg(country_code), sqlc.arg(instructions), sqlc.arg(hours),
    sqlc.arg(is_active), sqlc.arg(sort_order)
)
ON CONFLICT (id) DO UPDATE SET
    name         = EXCLUDED.name,
    street1      = EXCLUDED.street1,
    street2      = EXCLUDED.street2,
    city         = EXCLUDED.city,
    region       = EXCLUDED.region,
    postal_code  = EXCLUDED.postal_code,
    country_code = EXCLUDED.country_code,
    instructions = EXCLUDED.instructions,
    hours        = EXCLUDED.hours,
    is_active    = EXCLUDED.is_active,
    sort_order   = EXCLUDED.sort_order,
    updated_at   = NOW()
WHERE delivery_points.channel_id = EXCLUDED.channel_id
RETURNING *;

-- name: DeleteDeliveryPointsNotIn :exec
-- Drops the channel's points a save left out - the dashboard saves the whole list at once
DELETE FROM delivery_points
WHERE channel_id = sqlc.arg(channel_id) AND NOT (id = ANY(sqlc.arg(keep)::uuid[]));

-- ==================== Product delivery profiles ====================

-- name: GetDeliveryProfilesBySKUs :many
-- What checkout needs of each cart line's product to group it - whether it ships at all and which delivery kinds its type allows (NULL = every kind the channel offers). One round-trip for the whole cart, mirroring GetProductTaxClassesBySKUs
SELECT p.sku, p.id AS product_id, pt.id AS product_type_id, pt.is_shipping_required, pt.delivery_kinds, pt.name AS product_type_name
FROM catalog_products p
JOIN catalog_product_types pt ON pt.id = p.product_type_id
WHERE p.sku = ANY(sqlc.arg(skus)::text[]);

-- name: SetProductTypeDeliveryKinds :exec
UPDATE catalog_product_types
SET delivery_kinds = sqlc.narg(delivery_kinds)::text[],
    updated_at     = NOW()
WHERE id = sqlc.arg(id);

-- ==================== Order shipments ====================

-- name: InsertOrderShipment :one
INSERT INTO order_shipments (
    order_id, position, group_key, origin_location_id, delivery_kind, delivery_option_id, delivery_option_name,
    carrier_slug, method_code, method_name, amount_cents, estimated_days, ship_to, point, lines
)
VALUES (
    sqlc.arg(order_id), sqlc.arg(position), sqlc.arg(group_key), sqlc.narg(origin_location_id), sqlc.arg(delivery_kind),
    sqlc.narg(delivery_option_id), sqlc.arg(delivery_option_name), sqlc.arg(carrier_slug), sqlc.arg(method_code),
    sqlc.arg(method_name), sqlc.arg(amount_cents), sqlc.narg(estimated_days), sqlc.narg(ship_to), sqlc.narg(point),
    sqlc.arg(lines)
)
RETURNING *;

-- name: ListOrderShipments :many
SELECT * FROM order_shipments WHERE order_id = $1 ORDER BY position;

-- name: SetOrderFulfillmentNeed :exec
UPDATE orders
SET fulfillment_need = sqlc.arg(fulfillment_need),
    updated_at       = NOW()
WHERE id = sqlc.arg(id);
