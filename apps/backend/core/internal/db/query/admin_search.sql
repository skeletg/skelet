-- The dashboard's ⌘K lookup: a few light rows per kind, matched on a substring of what the admin typed, newest first.
-- No counts and none of a list page's loading - each OR branch has its own trigram index (admin_search_indexes)
-- A nil channel list is every channel; the caller passes the channels the admin may read

-- name: AdminSearchOrders :many
SELECT id, number, email, shipping_first_name, shipping_last_name
FROM orders
WHERE (sqlc.narg(channel_ids)::uuid[] IS NULL OR channel_id = ANY(sqlc.narg(channel_ids)::uuid[]))
  AND (number ILIKE '%' || escape_like(sqlc.arg(term)::text) || '%'
       OR email ILIKE '%' || escape_like(sqlc.arg(term)::text) || '%'
       OR (shipping_first_name || ' ' || shipping_last_name) ILIKE '%' || escape_like(sqlc.arg(term)::text) || '%')
ORDER BY created_at DESC
LIMIT sqlc.arg(row_limit);

-- name: AdminSearchCustomers :many
SELECT id, email, first_name, last_name
FROM customers
WHERE (sqlc.narg(channel_ids)::uuid[] IS NULL OR channel_id = ANY(sqlc.narg(channel_ids)::uuid[]))
  AND (email ILIKE '%' || escape_like(sqlc.arg(term)::text) || '%'
       OR (first_name || ' ' || last_name) ILIKE '%' || escape_like(sqlc.arg(term)::text) || '%')
ORDER BY created_at DESC
LIMIT sqlc.arg(row_limit);

-- name: AdminSearchProducts :many
-- Catalog rows are global, so no channel filter. The name is the admin's locale's, else whichever translation exists
SELECT p.id, p.sku,
       COALESCE(
         (SELECT c.name FROM catalog_product_content c WHERE c.product_id = p.id AND c.locale = sqlc.arg(locale)::text),
         (SELECT c.name FROM catalog_product_content c WHERE c.product_id = p.id ORDER BY c.locale LIMIT 1),
         '')::text AS name
FROM catalog_products p
WHERE p.id IN (
    SELECT s.id FROM catalog_products s WHERE s.sku ILIKE '%' || escape_like(sqlc.arg(term)::text) || '%'
    UNION
    SELECT n.product_id FROM catalog_product_content n WHERE n.name ILIKE '%' || escape_like(sqlc.arg(term)::text) || '%')
ORDER BY p.updated_at DESC
LIMIT sqlc.arg(row_limit);
