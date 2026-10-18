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
-- Total references across live-data columns - orders.currency_code and subscription_invoices.currency_code are intentionally excluded - those are snapshots; history must not block retiring a currency - gift cards, store credit and subscriptions carry no FK, so this count is all that stops them pointing at nothing
SELECT (
    (SELECT COUNT(*) FROM channels             WHERE currency_code         = UPPER($1)) +
    (SELECT COUNT(*) FROM catalog_products     WHERE default_currency_code = UPPER($1)) +
    (SELECT COUNT(*) FROM gift_cards           WHERE currency_code         = UPPER($1)) +
    (SELECT COUNT(*) FROM store_credit_entries WHERE currency_code         = UPPER($1)) +
    (SELECT COUNT(*) FROM subscriptions        WHERE currency_code         = UPPER($1))
)::bigint AS total;

-- name: CurrencyHasAmounts :one
-- Whether any stored minor-unit amount is denominated in the currency, history included - a decimals change would reinterpret every one of them
SELECT (
    EXISTS (SELECT 1 FROM channels              WHERE currency_code         = UPPER($1)) OR
    EXISTS (SELECT 1 FROM catalog_products      WHERE default_currency_code = UPPER($1)) OR
    EXISTS (SELECT 1 FROM gift_cards            WHERE currency_code         = UPPER($1)) OR
    EXISTS (SELECT 1 FROM store_credit_entries  WHERE currency_code         = UPPER($1)) OR
    EXISTS (SELECT 1 FROM subscriptions         WHERE currency_code         = UPPER($1)) OR
    EXISTS (SELECT 1 FROM subscription_invoices WHERE currency_code         = UPPER($1)) OR
    EXISTS (SELECT 1 FROM orders                WHERE currency_code         = UPPER($1))
)::boolean AS referenced;

-- name: CountChannelsUsingCurrency :one
SELECT COUNT(*)::bigint FROM channels WHERE currency_code = UPPER($1);

-- name: SetCurrencyRate :execrows
UPDATE currencies
SET exchange_rate   = $2,
    rate_updated_at = NOW(),
    updated_at      = NOW()
WHERE code = $1;
