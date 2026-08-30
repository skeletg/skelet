-- name: UpdateCountryIsoCodes :exec
-- Enrich an existing country with its alpha-3 + numeric-3 ISO codes
-- No-op when the country code isn't present
UPDATE countries
SET iso3_code = $2, numeric_code = $3, updated_at = NOW()
WHERE code = $1;

-- name: UpsertRegion :exec
-- Skips silently when the country isn't present (an install may enable
-- only a subset of countries) - the curated region set seeds only for
-- countries that exist
INSERT INTO regions (country_code, code, name, sort_order)
SELECT $1, $2, $3, $4
WHERE EXISTS (SELECT 1 FROM countries WHERE code = $1)
ON CONFLICT (country_code, code) DO UPDATE
    SET name = EXCLUDED.name, sort_order = EXCLUDED.sort_order, updated_at = NOW();

-- name: ListRegionsByCountry :many
SELECT * FROM regions
WHERE country_code = $1 AND is_active = TRUE
ORDER BY sort_order, name;
