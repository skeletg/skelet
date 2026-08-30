-- Read-only aggregate queries backing the sales/reporting dashboard
-- Every query is filtered by an inclusive [start_at, end_at] range on
-- orders.created_at and an optional channel-id set: when channel_ids is NULL
-- (superuser / global-scoped caller) no channel filter applies; otherwise rows
-- are restricted to that set - money columns are int64 cents; revenue metrics
-- exclude cancelled + draft orders and group by currency_code so multi-currency
-- channels never sum across currencies

-- name: SalesSummaryByCurrency :many
SELECT
    o.currency_code                                  AS currency_code,
    COUNT(*)::INTEGER                                AS order_count,
    COALESCE(SUM(o.total_amount), 0)::BIGINT         AS gross_revenue,
    COALESCE(SUM(o.subtotal_amount), 0)::BIGINT      AS subtotal_total,
    COALESCE(SUM(o.discount_amount), 0)::BIGINT      AS discount_total,
    COALESCE(SUM(o.tax_amount), 0)::BIGINT           AS tax_total,
    COALESCE(SUM(o.shipping_amount), 0)::BIGINT      AS shipping_total,
    COUNT(DISTINCT o.customer_id)::INTEGER           AS unique_customers
FROM orders o
WHERE o.status NOT IN ('cancelled', 'draft')
  AND o.created_at >= sqlc.arg(start_at)
  AND o.created_at <= sqlc.arg(end_at)
  AND (sqlc.arg(all_channels)::boolean OR o.channel_id = ANY(sqlc.arg(channel_ids)::uuid[]))
GROUP BY o.currency_code
ORDER BY gross_revenue DESC;

-- name: SalesTimeseries :many
SELECT
    date_trunc(sqlc.arg(bucket)::text, o.created_at)::timestamptz AS period,
    o.currency_code                            AS currency_code,
    COUNT(*)::INTEGER                          AS order_count,
    COALESCE(SUM(o.total_amount), 0)::BIGINT   AS revenue
FROM orders o
WHERE o.status NOT IN ('cancelled', 'draft')
  AND o.created_at >= sqlc.arg(start_at)
  AND o.created_at <= sqlc.arg(end_at)
  AND (sqlc.arg(all_channels)::boolean OR o.channel_id = ANY(sqlc.arg(channel_ids)::uuid[]))
GROUP BY period, o.currency_code
ORDER BY period;

-- name: SalesTopProducts :many
SELECT
    ol.product_sku                            AS product_sku,
    ol.product_name                           AS product_name,
    COALESCE(SUM(ol.quantity), 0)::BIGINT     AS units_sold,
    COALESCE(SUM(ol.total_price), 0)::BIGINT  AS revenue,
    COALESCE(SUM(ol.cost_cents * ol.quantity), 0)::BIGINT AS cost,
    COUNT(DISTINCT ol.order_id)::INTEGER      AS order_count
FROM order_lines ol
JOIN orders o ON o.id = ol.order_id
WHERE o.status NOT IN ('cancelled', 'draft')
  AND ol.removed_at IS NULL
  AND o.created_at >= sqlc.arg(start_at)
  AND o.created_at <= sqlc.arg(end_at)
  AND (sqlc.arg(all_channels)::boolean OR o.channel_id = ANY(sqlc.arg(channel_ids)::uuid[]))
GROUP BY ol.product_sku, ol.product_name
ORDER BY revenue DESC, units_sold DESC
LIMIT sqlc.arg(lim);

-- name: SalesForProductSku :one
-- One product's rollup, for the product detail sidebar - the top-N query can't answer it, since a
-- product outside the top N is indistinguishable from one that never sold
SELECT
    COALESCE(SUM(ol.quantity), 0)::BIGINT     AS units_sold,
    COALESCE(SUM(ol.total_price), 0)::BIGINT  AS revenue,
    COALESCE(SUM(ol.cost_cents * ol.quantity), 0)::BIGINT AS cost,
    COUNT(DISTINCT ol.order_id)::INTEGER      AS order_count
FROM order_lines ol
JOIN orders o ON o.id = ol.order_id
WHERE o.status NOT IN ('cancelled', 'draft')
  AND ol.removed_at IS NULL
  AND ol.product_sku = sqlc.arg(product_sku)
  AND o.created_at >= sqlc.arg(start_at)
  AND o.created_at <= sqlc.arg(end_at)
  AND (sqlc.arg(all_channels)::boolean OR o.channel_id = ANY(sqlc.arg(channel_ids)::uuid[]));

-- name: SalesByStatus :many
SELECT
    o.status                                 AS status,
    COUNT(*)::INTEGER                         AS order_count,
    COALESCE(SUM(o.total_amount), 0)::BIGINT  AS revenue
FROM orders o
WHERE o.created_at >= sqlc.arg(start_at)
  AND o.created_at <= sqlc.arg(end_at)
  AND (sqlc.arg(all_channels)::boolean OR o.channel_id = ANY(sqlc.arg(channel_ids)::uuid[]))
GROUP BY o.status
ORDER BY order_count DESC;

-- name: SalesRefundTotals :many
SELECT
    o.currency_code                            AS currency_code,
    COALESCE(SUM(t.amount_cents), 0)::BIGINT   AS refunded_amount,
    COUNT(*)::INTEGER                          AS refund_count
FROM order_payment_transactions t
JOIN orders o ON o.id = t.order_id
WHERE t.kind = 'refund'
  AND t.status = 'succeeded'
  AND t.created_at >= sqlc.arg(start_at)
  AND t.created_at <= sqlc.arg(end_at)
  AND (sqlc.arg(all_channels)::boolean OR o.channel_id = ANY(sqlc.arg(channel_ids)::uuid[]))
GROUP BY o.currency_code
ORDER BY refunded_amount DESC;

-- name: SalesCostByCurrency :many
-- COGS snapshot totals per currency (unit cost x quantity over active
-- lines) - lines without a snapshot (NULL cost_cents) contribute nothing;
-- margin consumers should treat cost as a floor, not an exact figure,
-- until the catalog carries costs for every SKU
SELECT
    o.currency_code                                            AS currency_code,
    COALESCE(SUM(ol.cost_cents * ol.quantity), 0)::BIGINT      AS cost_total
FROM order_lines ol
JOIN orders o ON o.id = ol.order_id
WHERE o.status NOT IN ('cancelled', 'draft')
  AND ol.removed_at IS NULL
  AND o.created_at >= sqlc.arg(start_at)
  AND o.created_at <= sqlc.arg(end_at)
  AND (sqlc.arg(all_channels)::boolean OR o.channel_id = ANY(sqlc.arg(channel_ids)::uuid[]))
GROUP BY o.currency_code;
