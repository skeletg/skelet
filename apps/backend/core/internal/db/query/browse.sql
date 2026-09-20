-- Commerce half of the browse-abandonment scan. browse_seen / browse_notified and the anon→customer resolution moved to
-- the signals db; only the cart/order exclusion (which needs commerce tables) stays here, fed candidate ids app-side

-- name: BrowseAbandonersExcluded :many
-- Commerce half of the browse-abandonment scan: of the passed candidates (their browse-seen/identity resolution ran in signals), the customers to EXCLUDE - those with an active non-empty cart (cart-abandonment owns them) or an order placed since their browse - paired arrays: customer_ids[i] browsed at browse_ats[i]
SELECT c.customer_id::uuid AS customer_id
FROM (
    SELECT unnest(sqlc.arg(customer_ids)::uuid[]) AS customer_id,
           unnest(sqlc.arg(browse_ats)::timestamptz[]) AS browse_at
) c
WHERE EXISTS (SELECT 1 FROM carts ca JOIN cart_lines cl ON cl.cart_id = ca.id WHERE ca.customer_id = c.customer_id AND ca.status = 'active')
   OR EXISTS (SELECT 1 FROM orders o WHERE o.customer_id = c.customer_id AND o.created_at >= c.browse_at);

