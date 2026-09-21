-- name: CreateCountry :one
INSERT INTO countries (
    code, name, native_name, emoji_flag, phone_code,
    default_currency_code, is_active, sort_order, metadata
) VALUES (
    UPPER($1), $2, $3, $4, $5,
    NULLIF(UPPER($6), ''), $7, $8, $9
)
RETURNING *;

-- name: GetCountryByCode :one
SELECT * FROM countries WHERE code = UPPER($1);

-- name: ListCountries :many
SELECT * FROM countries ORDER BY sort_order, code;

-- name: ListActiveCountries :many
SELECT * FROM countries WHERE is_active ORDER BY sort_order, code;

-- name: UpdateCountry :one
UPDATE countries
SET name                  = $2,
    native_name           = $3,
    emoji_flag            = $4,
    phone_code            = $5,
    default_currency_code = NULLIF(UPPER($6), ''),
    is_active             = $7,
    sort_order            = $8,
    metadata              = $9,
    updated_at            = NOW()
WHERE code = UPPER($1)
RETURNING *;

-- name: DeleteCountry :exec
DELETE FROM countries WHERE code = UPPER($1);

-- name: CountCountryReferences :one
-- Total references across live-data columns. orders.* country fields are intentionally excluded - they're snapshots; historical orders must not block disabling a country
SELECT (
    (SELECT COUNT(*) FROM addresses WHERE country_code         = UPPER($1)) +
    (SELECT COUNT(*) FROM locales   WHERE default_country_code = UPPER($1))
)::bigint AS total;
