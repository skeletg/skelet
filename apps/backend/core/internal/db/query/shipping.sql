-- Zones

-- name: CreateShippingZone :one
INSERT INTO shipping_zones (channel_id, code, name, sort_order, is_active)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: UpdateShippingZone :one
UPDATE shipping_zones
SET code = $2, name = $3, sort_order = $4, is_active = $5, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteShippingZone :exec
DELETE FROM shipping_zones WHERE id = $1;

-- name: GetShippingZone :one
SELECT * FROM shipping_zones WHERE id = $1;

-- name: ListShippingZonesByChannel :many
SELECT * FROM shipping_zones
WHERE channel_id = $1
ORDER BY sort_order, code;

-- name: FindZoneForCountry :one
-- Picks the first active zone for (store, country) - stable by sort_order then code so the result is deterministic when multiple zones overlap on the same country
SELECT z.*
FROM shipping_zones z
JOIN shipping_zone_countries c ON c.zone_id = z.id
WHERE z.channel_id = $1
  AND z.is_active = TRUE
  AND c.country_code = $2
ORDER BY z.sort_order, z.code
LIMIT 1;

-- Zone countries

-- name: AddCountryToZone :exec
INSERT INTO shipping_zone_countries (zone_id, country_code)
VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: RemoveCountryFromZone :exec
DELETE FROM shipping_zone_countries
WHERE zone_id = $1 AND country_code = $2;

-- name: ListCountriesForZone :many
SELECT country_code FROM shipping_zone_countries
WHERE zone_id = $1
ORDER BY country_code;

-- name: ClearZoneCountries :exec
DELETE FROM shipping_zone_countries WHERE zone_id = $1;

-- Methods

-- name: CreateShippingMethod :one
INSERT INTO shipping_methods (
    zone_id, code, name, calc_type, flat_cents, free_above_cents, sort_order, is_active, country_mode, delivery_kind, units_per_charge,
    estimated_days, delivers_on_weekends
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
RETURNING *;

-- name: UpdateShippingMethod :one
UPDATE shipping_methods
SET code = $2,
    name = $3,
    calc_type = $4,
    flat_cents = $5,
    free_above_cents = $6,
    sort_order = $7,
    is_active = $8,
    country_mode = $9,
    delivery_kind = $10,
    units_per_charge = $11,
    estimated_days = $12,
    delivers_on_weekends = $13,
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteShippingMethod :exec
DELETE FROM shipping_methods WHERE id = $1;

-- name: GetShippingMethod :one
SELECT * FROM shipping_methods WHERE id = $1;

-- name: GetShippingMethodByCode :one
SELECT * FROM shipping_methods WHERE zone_id = $1 AND code = $2;

-- name: ListShippingMethodsByZone :many
SELECT * FROM shipping_methods
WHERE zone_id = $1
ORDER BY sort_order, code;

-- name: ListActiveShippingMethodsByZone :many
SELECT * FROM shipping_methods
WHERE zone_id = $1 AND is_active = TRUE
ORDER BY sort_order, code;

-- Method rates per product type - a type listed here is priced by its own row, every other line pools into the method's default rate

-- name: ListMethodTypeRates :many
SELECT r.method_id, r.product_type_id, pt.name AS product_type_name, r.price_cents, r.units_per_charge, r.free_shipping_eligible
FROM shipping_method_type_rates r
JOIN catalog_product_types pt ON pt.id = r.product_type_id
WHERE r.method_id = ANY(sqlc.arg(method_ids)::uuid[])
ORDER BY r.method_id, pt.name;

-- name: ClearMethodTypeRates :exec
DELETE FROM shipping_method_type_rates WHERE method_id = $1;

-- name: InsertMethodTypeRate :exec
INSERT INTO shipping_method_type_rates (method_id, product_type_id, price_cents, units_per_charge, free_shipping_eligible)
VALUES ($1, $2, $3, $4, $5);

-- Method country allow-list (mirrors the zone-country queries above)

-- name: AddCountryToMethod :exec
INSERT INTO shipping_method_countries (method_id, country_code)
VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: ListCountriesForMethod :many
SELECT country_code FROM shipping_method_countries
WHERE method_id = $1
ORDER BY country_code;

-- name: ClearMethodCountries :exec
DELETE FROM shipping_method_countries WHERE method_id = $1;

-- Boxes (cartonization)

-- name: UpsertShippingBox :one
INSERT INTO shipping_boxes (channel_id, code, name, inner_length_mm, inner_width_mm, inner_height_mm, max_weight_grams, box_weight_grams, is_active, sort_order)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
ON CONFLICT (channel_id, code) DO UPDATE SET
    name = EXCLUDED.name,
    inner_length_mm = EXCLUDED.inner_length_mm,
    inner_width_mm = EXCLUDED.inner_width_mm,
    inner_height_mm = EXCLUDED.inner_height_mm,
    max_weight_grams = EXCLUDED.max_weight_grams,
    box_weight_grams = EXCLUDED.box_weight_grams,
    is_active = EXCLUDED.is_active,
    sort_order = EXCLUDED.sort_order,
    updated_at = NOW()
RETURNING *;

-- name: GetShippingBox :one
-- Single box by id - the admin delete path needs its channel to authorize
SELECT * FROM shipping_boxes WHERE id = $1;

-- name: DeleteShippingBox :exec
DELETE FROM shipping_boxes WHERE id = $1;

-- name: ListShippingBoxesByChannel :many
SELECT * FROM shipping_boxes WHERE channel_id = $1 ORDER BY sort_order, code;

-- name: ListActiveShippingBoxesByChannel :many
SELECT * FROM shipping_boxes WHERE channel_id = $1 AND is_active = TRUE ORDER BY sort_order, code;
