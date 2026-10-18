-- name: ListRegionsByCountry :many
SELECT * FROM regions
WHERE country_code = $1 AND is_active = TRUE
ORDER BY sort_order, name;
