-- name: CreateCurrency :one
INSERT INTO currencies (
    code, name, symbol, decimals, symbol_position,
    is_active, sort_order, metadata
) VALUES (
    UPPER($1), $2, $3, $4, $5,
    $6, $7, $8
)
RETURNING *;

-- name: GetCurrencyByCode :one
SELECT * FROM currencies WHERE code = UPPER($1);

-- name: ListCurrencies :many
SELECT * FROM currencies ORDER BY sort_order, code;

-- name: ListActiveCurrencies :many
SELECT * FROM currencies WHERE is_active ORDER BY sort_order, code;

-- name: UpdateCurrency :one
UPDATE currencies
SET name            = $2,
    symbol          = $3,
    decimals        = $4,
    symbol_position = $5,
    is_active       = $6,
    sort_order      = $7,
    metadata        = $8,
    updated_at      = NOW()
WHERE code = UPPER($1)
RETURNING *;

-- name: DeleteCurrency :exec
DELETE FROM currencies WHERE code = UPPER($1);

-- name: CountCurrencyReferences :one
-- Total references across live-data columns - orders.currency_code is
-- intentionally excluded - that column is a snapshot; historical orders
-- must not block disabling a currency - phase C adds FK constraints so
-- the live values can't drift; the count is here for the friendlier
-- "N references; resolve first" delete error
SELECT (
    (SELECT COUNT(*) FROM channels WHERE currency_code         = UPPER($1)) +
    (SELECT COUNT(*) FROM catalog_products    WHERE default_currency_code = UPPER($1))
)::bigint AS total;

-- name: SetCurrencyRate :exec
UPDATE currencies
SET exchange_rate   = $2,
    rate_updated_at = NOW(),
    updated_at      = NOW()
WHERE code = $1;
