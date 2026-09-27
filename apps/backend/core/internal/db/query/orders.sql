-- name: CreateOrder :one
INSERT INTO orders (
    id, customer_id, email, channel_id, status, payment_status,
    billing_first_name, billing_last_name, billing_company,
    billing_street_1, billing_street_2, billing_city, billing_state,
    billing_postal_code, billing_country, billing_phone,
    shipping_first_name, shipping_last_name, shipping_company,
    shipping_street_1, shipping_street_2, shipping_city, shipping_state,
    shipping_postal_code, shipping_country, shipping_phone,
    subtotal_amount, shipping_amount, discount_amount, tax_amount, total_amount,
    currency_code, customer_note, payment_provider, shipping_method_id,
    shipping_carrier_slug, shipping_method_code, po_number, shipping_tax_amount, anon_id
) VALUES (
    $1, $2, $3, $4, $5, $6,
    $7, $8, $9, $10, $11, $12, $13, $14, $15, $16,
    $17, $18, $19, $20, $21, $22, $23, $24, $25, $26,
    $27, $28, $29, $30, $31,
    $32, $33, $34, $35,
    $36, $37, $38, $39, $40
) RETURNING *;

-- name: GetOrderByID :one
SELECT * FROM orders WHERE id = $1 LIMIT 1;

-- name: ListOrdersPaged :many
-- Every channel's page - the scoped caller has its own statement, so neither shares a plan that reads the whole created_at index
-- SELECT * (not window-count) so the repo maps rows through toOrder verbatim - CountOrdersPaged shares the WHERE and must stay in lockstep
SELECT * FROM orders o
WHERE  TRUE
  AND  (sqlc.arg(search)::text = '' OR
        o.number ILIKE '%' || sqlc.arg(search)::text || '%' OR
        o.email ILIKE '%' || sqlc.arg(search)::text || '%' OR
        o.shipping_last_name ILIKE '%' || sqlc.arg(search)::text || '%' OR
        o.billing_last_name ILIKE '%' || sqlc.arg(search)::text || '%')
  AND  (sqlc.arg(all_statuses)::boolean OR o.status = ANY(sqlc.arg(statuses)::text[]))
  AND  (sqlc.arg(all_payment)::boolean OR o.payment_status = ANY(sqlc.arg(payment_statuses)::text[]))
ORDER  BY o.created_at DESC
LIMIT  sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: CountOrdersPaged :one
SELECT COUNT(*) FROM orders o
WHERE  TRUE
  AND  (sqlc.arg(search)::text = '' OR
        o.number ILIKE '%' || sqlc.arg(search)::text || '%' OR
        o.email ILIKE '%' || sqlc.arg(search)::text || '%' OR
        o.shipping_last_name ILIKE '%' || sqlc.arg(search)::text || '%' OR
        o.billing_last_name ILIKE '%' || sqlc.arg(search)::text || '%')
  AND  (sqlc.arg(all_statuses)::boolean OR o.status = ANY(sqlc.arg(statuses)::text[]))
  AND  (sqlc.arg(all_payment)::boolean OR o.payment_status = ANY(sqlc.arg(payment_statuses)::text[]));

-- name: ListOrdersPagedInChannels :many
-- Scoped in SQL, so LIMIT runs after the permission filter
SELECT * FROM orders o
WHERE  o.channel_id = ANY(sqlc.arg(channel_ids)::uuid[])
  AND  (sqlc.arg(search)::text = '' OR
        o.number ILIKE '%' || sqlc.arg(search)::text || '%' OR
        o.email ILIKE '%' || sqlc.arg(search)::text || '%' OR
        o.shipping_last_name ILIKE '%' || sqlc.arg(search)::text || '%' OR
        o.billing_last_name ILIKE '%' || sqlc.arg(search)::text || '%')
  AND  (sqlc.arg(all_statuses)::boolean OR o.status = ANY(sqlc.arg(statuses)::text[]))
  AND  (sqlc.arg(all_payment)::boolean OR o.payment_status = ANY(sqlc.arg(payment_statuses)::text[]))
ORDER  BY o.created_at DESC
LIMIT  sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: CountOrdersPagedInChannels :one
SELECT COUNT(*) FROM orders o
WHERE  o.channel_id = ANY(sqlc.arg(channel_ids)::uuid[])
  AND  (sqlc.arg(search)::text = '' OR
        o.number ILIKE '%' || sqlc.arg(search)::text || '%' OR
        o.email ILIKE '%' || sqlc.arg(search)::text || '%' OR
        o.shipping_last_name ILIKE '%' || sqlc.arg(search)::text || '%' OR
        o.billing_last_name ILIKE '%' || sqlc.arg(search)::text || '%')
  AND  (sqlc.arg(all_statuses)::boolean OR o.status = ANY(sqlc.arg(statuses)::text[]))
  AND  (sqlc.arg(all_payment)::boolean OR o.payment_status = ANY(sqlc.arg(payment_statuses)::text[]));

-- name: ListOrdersAwaitingPaymentSync :many
-- The reconciliation sweep's worklist: gateway orders parked unpaid with a provider reference to ask
-- about - the lower bound stops ancient abandoned orders polling gateways forever
SELECT * FROM orders
WHERE  payment_status = 'unpaid'
  AND  payment_provider <> ''
  AND  payment_reference <> ''
  AND  status <> 'cancelled'
  AND  updated_at BETWEEN sqlc.arg(not_before) AND sqlc.arg(older_than)
ORDER  BY updated_at
LIMIT  sqlc.arg(max_rows);

-- name: ListOrdersByCustomer :many
SELECT * FROM orders WHERE customer_id = $1 ORDER BY created_at DESC;

-- name: GetGuestOrder :one
-- (id, email) lookup for unauthenticated order detail - UUID + email together act as a bearer credential - anyone who knows both can view the order - guest orders only (customer_id IS NULL); customer orders are accessed via the authenticated myOrder() path
SELECT * FROM orders
WHERE  id = $1 AND email = $2 AND customer_id IS NULL
LIMIT 1;

-- name: ClaimGuestOrdersByEmail :execrows
-- Attach every guest order placed on this channel under the given email to the freshly-registered customer - called from the customer.registered subscriber so the customer's order history on /account/orders shows up immediately after sign-up
UPDATE orders
SET    customer_id = $1, updated_at = NOW()
WHERE  customer_id IS NULL
  AND  email = $2
  AND  channel_id = $3;

-- name: UpdateOrderStatusFrom :one
-- Guarded by the status the caller validated against - validTransition runs on a read taken before the transaction (and before the pre-commit hooks, which do network I/O), so without this precondition two concurrent cancels both pass the check and both commit, enqueueing two order.cancelled events and reversing every tender twice No row back means someone else moved the order first - that's a conflict, not an error
UPDATE orders
SET    status = sqlc.arg(new_status), updated_at = NOW()
WHERE  id = sqlc.arg(id) AND status = sqlc.arg(from_status)
RETURNING *;

-- name: HoldOrderFrom :one
-- Move an order to on_hold while remembering where it came from, so releasing restores that status instead of promoting a pending order to confirmed Guarded on the read status for the same reason as UpdateOrderStatusFrom: a racing second hold would otherwise stamp held_from_status = 'on_hold' and wedge the release
UPDATE orders
SET    status = 'on_hold', held_from_status = status, updated_at = NOW()
WHERE  id = sqlc.arg(id) AND status = sqlc.arg(from_status)
RETURNING *;

-- name: ReleaseOrderToHeldFrom :one
-- Release a hold back to the remembered status, defaulting to confirmed for
-- rows held before held_from_status existed
UPDATE orders
SET    status = COALESCE(NULLIF(held_from_status, ''), 'confirmed'),
       held_from_status = '',
       updated_at = NOW()
WHERE  id = $1 AND status = 'on_hold'
RETURNING *;

-- name: UpdateOrderTracking :one
-- Stamp carrier + tracking on the order - called inside the fulfill
-- tx alongside UpdateOrderStatusFrom so the row reflects both at once
UPDATE orders
SET    carrier = $2, tracking_number = $3, updated_at = NOW()
WHERE  id = $1
RETURNING *;

-- name: SetOrderGiftCardTender :one
-- Records the cents covered by gift cards + the resulting payment status, inside the placement tx - the order total is unchanged (gift cards are a tender, not a discount); amount owed to the provider is total - this
UPDATE orders
SET    gift_card_amount = $2, payment_status = $3, updated_at = NOW()
WHERE  id = $1
RETURNING *;

-- name: SetOrderPaymentState :one
-- Updates payment status + gateway reference together (authorize stores the reference; capture/refund/void transitions re-pass the existing one) - the caller computes the next status idempotently
UPDATE orders
SET    payment_status = $2, payment_reference = $3, updated_at = NOW()
WHERE  id = $1
RETURNING *;

-- Optional explicit id: checkout passes the CART line's id so the inventory reservations written earlier in the placement tx (they link order_line_id before the line row exists - FK is deferred) resolve to the real order line - NULL generates one (draft/manual orders)
-- name: CreateOrderLine :one
-- is_gift marks a promotion free gift: zero-priced, stock reserved like any other line, but excluded from pro-rata refunds since the customer paid nothing for it
INSERT INTO order_lines (id, order_id, product_name, product_sku, unit_price, quantity, total_price, cost_cents, is_gift)
VALUES (COALESCE(sqlc.narg(id), uuidv7()), sqlc.arg(order_id), sqlc.arg(product_name), sqlc.arg(product_sku), sqlc.arg(unit_price), sqlc.arg(quantity), sqlc.arg(total_price), sqlc.arg(cost_cents), sqlc.arg(is_gift))
RETURNING *;

-- name: ListOrderLinesByOrder :many
-- Active lines only: an edit soft-deletes a removed line (removed_at) so its
-- FKs + audit survive, but it no longer counts toward the order
SELECT * FROM order_lines WHERE order_id = $1 AND removed_at IS NULL;

-- name: UpdateOrderLineQuantity :one
-- Edit a line's quantity + recomputed line total - guarded to active lines
UPDATE order_lines
SET    quantity = $2, total_price = $3
WHERE  id = $1 AND removed_at IS NULL
RETURNING *;

-- name: SoftDeleteOrderLine :one
-- Remove a line from an order without deleting the row (preserves
-- reservation/return FKs + the immutable record)
UPDATE order_lines
SET    removed_at = NOW()
WHERE  id = $1 AND removed_at IS NULL
RETURNING *;

-- name: UpdateOrderAmounts :one
-- Persist recomputed money after an edit. payment_status is left untouched -
-- the edit records payment drift separately rather than silently re-settling
UPDATE orders
SET    subtotal_amount = $2, shipping_amount = $3, discount_amount = $4,
       tax_amount = $5, total_amount = $6, shipping_tax_amount = $7, updated_at = NOW()
WHERE  id = $1
RETURNING *;

-- name: UpdateOrderAddresses :one
UPDATE orders
SET billing_first_name = $2, billing_last_name = $3, billing_company = $4,
    billing_street_1 = $5, billing_street_2 = $6, billing_city = $7,
    billing_state = $8, billing_postal_code = $9, billing_country = $10,
    billing_phone = $11,
    shipping_first_name = $12, shipping_last_name = $13, shipping_company = $14,
    shipping_street_1 = $15, shipping_street_2 = $16, shipping_city = $17,
    shipping_state = $18, shipping_postal_code = $19, shipping_country = $20,
    shipping_phone = $21,
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: CreateOrderEvent :one
INSERT INTO order_events (order_id, type, message, actor_email)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ListOrderEventsByOrder :many
SELECT * FROM order_events WHERE order_id = $1 ORDER BY created_at ASC;

-- name: CountOrdersByCustomer :one
-- Used by customer.Service.Delete to refuse deletion of customers with post-purchase history - the query physically lives in orders.sql for semantic clarity even though customer.Repository calls it - sqlc generates one `db` package so the file-split is cosmetic
SELECT COUNT(*) FROM orders WHERE customer_id = $1;

-- name: ListOrderLineOptionsByOrderLineIDs :many
-- Read-back of the custom-option snapshot captured at placement, batched by
-- order line - powers OrderLine.options (fulfillment + customer order view)
SELECT * FROM order_line_options
WHERE order_line_id = ANY(sqlc.arg(order_line_ids)::uuid[])
ORDER BY order_line_id, sort_order, created_at;

-- name: LockOrderForPayment :exec
-- Serializes payment intents per order - an aggregate can't be locked, so the owning order row is taken before the ledger is summed - without it two concurrent captures read the same totals and both pass the guard
SELECT 1 FROM orders WHERE id = $1 FOR UPDATE;

-- name: InsertOrderTaxLine :exec
-- One row per tax rule that touched the order - name and rate are snapshots so
-- the record survives the rule being edited or deleted
INSERT INTO order_tax_lines (order_id, rule_id, name, rate_bps, amount_cents)
VALUES ($1, $2, $3, $4, $5);

-- name: ListOrderTaxLines :many
SELECT * FROM order_tax_lines WHERE order_id = $1 ORDER BY amount_cents DESC, name;

-- name: DeleteOrderTaxLines :exec
-- A pre-fulfillment edit recomputes tax from scratch, so the old breakdown goes
DELETE FROM order_tax_lines WHERE order_id = $1;
