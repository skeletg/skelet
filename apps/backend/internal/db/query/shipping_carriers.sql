-- name: UpsertShippingCarrier :one
-- Idempotent insert used by SyncCarriers() at boot to ensure every
-- (channel, registered_carrier_slug) pair has a row - ON CONFLICT preserves the
-- admin-set is_enabled flag - registration must not silently re-enable a
-- carrier an admin disabled
INSERT INTO shipping_carriers (channel_id, carrier_slug, is_enabled, display_name)
VALUES ($1, $2, $3, $4)
ON CONFLICT (channel_id, carrier_slug) DO UPDATE
SET display_name = EXCLUDED.display_name,
    updated_at   = NOW()
RETURNING *;

-- name: GetShippingCarrier :one
SELECT * FROM shipping_carriers
WHERE channel_id = $1 AND carrier_slug = $2;

-- name: ListShippingCarriersByChannel :many
SELECT * FROM shipping_carriers
WHERE channel_id = $1
ORDER BY sort_order, carrier_slug;

-- name: ListEnabledShippingCarriersByChannel :many
SELECT * FROM shipping_carriers
WHERE channel_id = $1 AND is_enabled = TRUE
ORDER BY sort_order, carrier_slug;

-- name: SetShippingCarrierEnabled :one
UPDATE shipping_carriers
SET is_enabled = $3, updated_at = NOW()
WHERE channel_id = $1 AND carrier_slug = $2
RETURNING *;

-- name: SetShippingCarrierSortOrder :one
UPDATE shipping_carriers
SET sort_order = $3, updated_at = NOW()
WHERE channel_id = $1 AND carrier_slug = $2
RETURNING *;

-- name: SetShippingCarrierCountryMode :one
UPDATE shipping_carriers
SET country_mode = $3, updated_at = NOW()
WHERE channel_id = $1 AND carrier_slug = $2
RETURNING *;

-- Per-carrier ship-to country allow-list

-- name: AddCountryToShippingCarrier :exec
INSERT INTO shipping_carrier_countries (carrier_id, country_code)
VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: ListCountriesForShippingCarrier :many
SELECT country_code FROM shipping_carrier_countries
WHERE carrier_id = $1
ORDER BY country_code;

-- name: ClearShippingCarrierCountries :exec
DELETE FROM shipping_carrier_countries WHERE carrier_id = $1;
