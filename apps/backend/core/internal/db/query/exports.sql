-- Back-office CSV export feeds (customers + accounting-shaped order lines)

-- name: ExportCustomerRows :many
-- One keyset page: rows strictly after (after_created_at, after_id), so a zero cursor starts at the beginning
SELECT c.id, c.email, c.first_name, c.last_name, c.phone, c.is_active,
       c.is_email_verified, c.created_at, c.channel_id,
       COALESCE(g.slug, '')::text AS group_slug
FROM customers c
LEFT JOIN customer_groups g ON g.id = c.customer_group_id
WHERE (sqlc.arg(all_channels)::boolean OR c.channel_id = ANY(sqlc.arg(channel_ids)::uuid[]))
  AND (c.created_at, c.id) > (sqlc.arg(after_created_at)::timestamptz, sqlc.arg(after_id)::uuid)
ORDER BY c.created_at, c.id
LIMIT sqlc.arg(page_size);

-- name: ExportOrderRows :many
-- end_at is exclusive; paged on (o.created_at, o.id, ol.id) like the customer export
SELECT o.id AS order_id, ol.id AS line_id, o.number, o.created_at, o.status, o.payment_status,
       COALESCE(o.payment_provider, '')::text AS payment_provider,
       o.email, o.currency_code,
       o.subtotal_amount, o.discount_amount, o.shipping_amount,
       o.tax_amount, o.total_amount, o.gift_card_amount,
       ol.product_sku, ol.product_name, ol.quantity, ol.unit_price,
       ol.total_price, ol.cost_cents,
       COALESCE(opts.options, '')::text AS line_options
FROM orders o
JOIN order_lines ol ON ol.order_id = o.id AND ol.removed_at IS NULL
LEFT JOIN LATERAL (
    SELECT string_agg(
        olo.option_title || ': ' ||
        COALESCE(NULLIF(olo.value_label, ''), NULLIF(olo.text_value, ''), 'file'),
        ' | ' ORDER BY olo.created_at
    ) AS options
    FROM order_line_options olo
    WHERE olo.order_line_id = ol.id
) opts ON TRUE
WHERE (sqlc.arg(all_channels)::boolean OR o.channel_id = ANY(sqlc.arg(channel_ids)::uuid[]))
  AND (sqlc.arg(all_statuses)::boolean OR o.status = ANY(sqlc.arg(statuses)::text[]))
  AND o.created_at >= sqlc.arg(start_at)
  AND o.created_at < sqlc.arg(end_at)
  AND (o.created_at, o.id, ol.id) > (sqlc.arg(after_created_at)::timestamptz, sqlc.arg(after_order_id)::uuid, sqlc.arg(after_line_id)::uuid)
ORDER BY o.created_at, o.id, ol.id
LIMIT sqlc.arg(page_size);

-- name: SalesVelocity30dBySKU :many
-- Units sold in the trailing 30 days per channel - feeds the search
-- ranker's sales_30d signal at index time
SELECT o.channel_id, COALESCE(SUM(ol.quantity), 0)::double precision AS units
FROM order_lines ol
JOIN orders o ON o.id = ol.order_id
WHERE ol.product_sku = $1
  AND o.created_at > NOW() - INTERVAL '30 days'
  AND o.status NOT IN ('cancelled', 'draft')
  AND ol.removed_at IS NULL
GROUP BY o.channel_id;

-- name: SalesVelocity30dBySKUs :many
-- Batched sibling of SalesVelocity30dBySKU for the bulk reindex: one query per page of products rather than one per product - SKUs with no sales in the window are simply absent from the result, same as the single-SKU form returning none
SELECT ol.product_sku, o.channel_id, COALESCE(SUM(ol.quantity), 0)::double precision AS units
FROM order_lines ol
JOIN orders o ON o.id = ol.order_id
WHERE ol.product_sku = ANY(sqlc.arg(skus)::text[])
  AND o.created_at > NOW() - INTERVAL '30 days'
  AND o.status NOT IN ('cancelled', 'draft')
  AND ol.removed_at IS NULL
GROUP BY ol.product_sku, o.channel_id;
