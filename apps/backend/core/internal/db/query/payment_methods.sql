-- name: UpsertPaymentMethod :one
-- Idempotent insert used by payment.Sync() at boot to ensure every (channel, registered_provider_slug) pair has a row - ON CONFLICT preserves the admin-set is_enabled flag - registration must not silently re-enable a method an admin disabled
INSERT INTO payment_methods (channel_id, provider_slug, display_name)
VALUES ($1, $2, $3)
ON CONFLICT (channel_id, provider_slug) DO UPDATE
SET display_name = EXCLUDED.display_name,
    updated_at   = NOW()
RETURNING *;

-- name: GetPaymentMethod :one
SELECT * FROM payment_methods
WHERE channel_id = $1 AND provider_slug = $2;

-- name: ListPaymentMethodsByChannel :many
SELECT * FROM payment_methods
WHERE channel_id = $1
ORDER BY sort_order, provider_slug;

-- name: ListEnabledPaymentMethodsByChannel :many
SELECT * FROM payment_methods
WHERE channel_id = $1 AND is_enabled = TRUE
ORDER BY sort_order, provider_slug;

-- name: ListPaymentMethodsByProvider :many
-- All (channel, payment_method) rows for one provider - used by
-- admin UIs that group providers across stores
SELECT * FROM payment_methods
WHERE provider_slug = $1
ORDER BY channel_id;

-- name: SetPaymentMethodEnabled :one
UPDATE payment_methods
SET is_enabled = $3, updated_at = NOW()
WHERE channel_id = $1 AND provider_slug = $2
RETURNING *;

-- name: SetPaymentMethodSortOrder :one
UPDATE payment_methods
SET sort_order = $3, updated_at = NOW()
WHERE channel_id = $1 AND provider_slug = $2
RETURNING *;

-- name: SetPaymentMethodCountryMode :one
UPDATE payment_methods
SET country_mode = $3, updated_at = NOW()
WHERE channel_id = $1 AND provider_slug = $2
RETURNING *;

-- Per-method country allow-list

-- name: AddCountryToPaymentMethod :exec
INSERT INTO payment_method_countries (payment_method_id, country_code)
VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: ListCountriesForPaymentMethod :many
SELECT country_code FROM payment_method_countries
WHERE payment_method_id = $1
ORDER BY country_code;

-- name: ClearPaymentMethodCountries :exec
DELETE FROM payment_method_countries WHERE payment_method_id = $1;
