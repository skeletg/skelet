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
    shipping_carrier_slug, shipping_method_code, po_number, shipping_tax_amount, anon_id, business_ids, assisted_by, metadata,
    locale, tax_inclusive, checkout_fields, number, seller, fees
) VALUES (
    $1, $2, $3, $4, $5, $6,
    $7, $8, $9, $10, $11, $12, $13, $14, $15, $16,
    $17, $18, $19, $20, $21, $22, $23, $24, $25, $26,
    $27, $28, $29, $30, $31,
    $32, $33, $34, $35,
    $36, $37, $38, $39, $40, $41, $42, $43,
    $44, $45, $46, $47, $48, $49
)
-- A number taken since it was checked inserts nothing rather than aborting the transaction, so the caller can name
-- the order again
ON CONFLICT (number) DO NOTHING
RETURNING *;

-- name: NextOrderNumber :one
-- One statement, outside the placement transaction, so a busy store never queues on the row
INSERT INTO order_number_counters (scope, period, next_value) VALUES ($1, $2, 2)
ON CONFLICT (scope, period) DO UPDATE SET next_value = order_number_counters.next_value + 1
RETURNING (next_value - 1)::bigint AS number;

-- name: OrderNumberTaken :one
SELECT EXISTS (SELECT 1 FROM orders WHERE number = $1) AS taken;

-- name: DefaultOrderNumber :one
SELECT next_order_number()::text AS number;

-- name: GetOrderByID :one
SELECT * FROM orders WHERE id = $1 LIMIT 1;

-- name: ListOrdersPaged :many
-- Every channel's page - the scoped caller has its own statement, so neither shares a plan that reads the whole created_at index
-- SELECT * (not window-count) so the repo maps rows through toOrder verbatim - CountOrdersPaged shares the WHERE and must stay in lockstep
SELECT * FROM orders o
WHERE  TRUE
  AND  (sqlc.arg(search)::text = '' OR
        o.number ILIKE '%' || escape_like(sqlc.arg(search)::text) || '%' OR
        o.email ILIKE '%' || escape_like(sqlc.arg(search)::text) || '%' OR
        o.shipping_last_name ILIKE '%' || escape_like(sqlc.arg(search)::text) || '%' OR
        o.billing_last_name ILIKE '%' || escape_like(sqlc.arg(search)::text) || '%')
  AND  (sqlc.arg(all_statuses)::boolean OR o.status = ANY(sqlc.arg(statuses)::text[]))
  AND  (sqlc.arg(all_payment)::boolean OR o.payment_status = ANY(sqlc.arg(payment_statuses)::text[]))
  AND  (sqlc.arg(all_stages)::boolean OR o.stage = ANY(sqlc.arg(stages)::text[]))
  AND  (NOT sqlc.arg(restrict_ids)::boolean OR o.id = ANY(sqlc.arg(only_ids)::uuid[]))
ORDER  BY o.created_at DESC
LIMIT  sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: CountOrdersPaged :one
SELECT COUNT(*) FROM orders o
WHERE  TRUE
  AND  (sqlc.arg(search)::text = '' OR
        o.number ILIKE '%' || escape_like(sqlc.arg(search)::text) || '%' OR
        o.email ILIKE '%' || escape_like(sqlc.arg(search)::text) || '%' OR
        o.shipping_last_name ILIKE '%' || escape_like(sqlc.arg(search)::text) || '%' OR
        o.billing_last_name ILIKE '%' || escape_like(sqlc.arg(search)::text) || '%')
  AND  (sqlc.arg(all_statuses)::boolean OR o.status = ANY(sqlc.arg(statuses)::text[]))
  AND  (sqlc.arg(all_payment)::boolean OR o.payment_status = ANY(sqlc.arg(payment_statuses)::text[]))
  AND  (sqlc.arg(all_stages)::boolean OR o.stage = ANY(sqlc.arg(stages)::text[]))
  AND  (NOT sqlc.arg(restrict_ids)::boolean OR o.id = ANY(sqlc.arg(only_ids)::uuid[]));

-- name: ListOrdersPagedInChannels :many
-- Scoped in SQL, so LIMIT runs after the permission filter
SELECT * FROM orders o
WHERE  o.channel_id = ANY(sqlc.arg(channel_ids)::uuid[])
  AND  (sqlc.arg(search)::text = '' OR
        o.number ILIKE '%' || escape_like(sqlc.arg(search)::text) || '%' OR
        o.email ILIKE '%' || escape_like(sqlc.arg(search)::text) || '%' OR
        o.shipping_last_name ILIKE '%' || escape_like(sqlc.arg(search)::text) || '%' OR
        o.billing_last_name ILIKE '%' || escape_like(sqlc.arg(search)::text) || '%')
  AND  (sqlc.arg(all_statuses)::boolean OR o.status = ANY(sqlc.arg(statuses)::text[]))
  AND  (sqlc.arg(all_payment)::boolean OR o.payment_status = ANY(sqlc.arg(payment_statuses)::text[]))
  AND  (sqlc.arg(all_stages)::boolean OR o.stage = ANY(sqlc.arg(stages)::text[]))
  AND  (NOT sqlc.arg(restrict_ids)::boolean OR o.id = ANY(sqlc.arg(only_ids)::uuid[]))
ORDER  BY o.created_at DESC
LIMIT  sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: CountOrdersPagedInChannels :one
SELECT COUNT(*) FROM orders o
WHERE  o.channel_id = ANY(sqlc.arg(channel_ids)::uuid[])
  AND  (sqlc.arg(search)::text = '' OR
        o.number ILIKE '%' || escape_like(sqlc.arg(search)::text) || '%' OR
        o.email ILIKE '%' || escape_like(sqlc.arg(search)::text) || '%' OR
        o.shipping_last_name ILIKE '%' || escape_like(sqlc.arg(search)::text) || '%' OR
        o.billing_last_name ILIKE '%' || escape_like(sqlc.arg(search)::text) || '%')
  AND  (sqlc.arg(all_statuses)::boolean OR o.status = ANY(sqlc.arg(statuses)::text[]))
  AND  (sqlc.arg(all_payment)::boolean OR o.payment_status = ANY(sqlc.arg(payment_statuses)::text[]))
  AND  (sqlc.arg(all_stages)::boolean OR o.stage = ANY(sqlc.arg(stages)::text[]))
  AND  (NOT sqlc.arg(restrict_ids)::boolean OR o.id = ANY(sqlc.arg(only_ids)::uuid[]));

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

-- name: TrackOrderLookup :one
-- Track-order lookup by what's on the receipt - the number is guessable, so email and postcode must both match, compared the way shoppers retype them (any case, postcode spacing) Matches guest AND customer orders; the caller hands out only the id and the stored email, and a customer order still opens only through the signed-in myOrder path
SELECT id, channel_id, email, (customer_id IS NOT NULL)::bool AS has_account FROM orders
WHERE  number = upper(btrim(sqlc.arg(number)::text))
  AND  lower(btrim(email)) = lower(btrim(sqlc.arg(email)::text))
  AND  upper(replace(COALESCE(NULLIF(shipping_postal_code, ''), billing_postal_code), ' ', ''))
       = upper(replace(sqlc.arg(postal_code)::text, ' ', ''));

-- name: ClaimGuestOrdersByEmail :execrows
-- Attach every guest order placed on this channel under the given email to the customer who just verified it - called from the customer.email_verified subscriber, never on registration, which proves nothing about owning the address
-- Order emails are stored as typed, so the match normalizes them the way customer emails already are
UPDATE orders
SET    customer_id = $1, updated_at = NOW()
WHERE  customer_id IS NULL
  AND  lower(btrim(email)) = $2
  AND  channel_id = $3;

-- name: UpdateOrderStatusFrom :one
-- Guarded by the status the caller validated against - validTransition runs on a read taken before the transaction (and before the pre-commit hooks, which do network I/O), so without this precondition two concurrent cancels both pass the check and both commit, enqueueing two order.cancelled events and reversing every tender twice No row back means someone else moved the order first - that's a conflict, not an error
UPDATE orders
SET    status = sqlc.arg(new_status), updated_at = NOW()
WHERE  id = sqlc.arg(id) AND status = sqlc.arg(from_status)
RETURNING *;

-- name: SetOrderStage :exec
UPDATE orders SET stage = sqlc.arg(stage), updated_at = NOW() WHERE id = sqlc.arg(id);

-- name: SetOrderStageFrom :execrows
-- Guarded on the status and stage read, so two moves racing on one order can't both land
UPDATE orders
SET    stage = sqlc.arg(stage), updated_at = NOW()
WHERE  id = sqlc.arg(id) AND status = sqlc.arg(from_status) AND stage = sqlc.arg(from_stage);

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

-- name: SetOrderPaymentStateFrom :one
-- Only when the status is still what the caller read - the sync sweep and a webhook settling at once let the slower one write authorized over paid
UPDATE orders
SET    payment_status = sqlc.arg(payment_status), payment_reference = sqlc.arg(payment_reference), updated_at = NOW()
WHERE  id = sqlc.arg(id) AND payment_status = sqlc.arg(from_status)
RETURNING *;

-- name: SetOrderPaymentReference :one
-- The reference alone - parking a 3DS handle used to write back the status read at placement, over whatever a webhook had set since
UPDATE orders SET payment_reference = $2, updated_at = NOW() WHERE id = $1
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
-- is_custom marks an admin's free-form charge from an offer - no catalog product, so nothing to reserve or ship, taxed by its own tax_class
INSERT INTO order_lines (id, order_id, product_name, product_sku, unit_price, quantity, total_price, cost_cents, is_gift, is_custom, tax_class, "position", discount_cents, tax_cents,
                         price_source, price_set_by, price_reason, list_unit_price)
VALUES (COALESCE(sqlc.narg(id), uuidv7()), sqlc.arg(order_id), sqlc.arg(product_name), sqlc.arg(product_sku), sqlc.arg(unit_price), sqlc.arg(quantity), sqlc.arg(total_price), sqlc.arg(cost_cents), sqlc.arg(is_gift), sqlc.arg(is_custom), sqlc.arg(tax_class), sqlc.arg(position), sqlc.arg(discount_cents), sqlc.arg(tax_cents),
        sqlc.arg(price_source), sqlc.narg(price_set_by), sqlc.arg(price_reason), sqlc.narg(list_unit_price))
RETURNING *;

-- name: ListOrderLinesByOrder :many
-- Active lines only: an edit soft-deletes a removed line (removed_at) so its
-- FKs + audit survive, but it no longer counts toward the order. In the order they were bought - an edited row
-- otherwise comes back wherever the table put its new version
SELECT * FROM order_lines WHERE order_id = $1 AND removed_at IS NULL ORDER BY "position", created_at, id;

-- name: SetOrderLineMoney :exec
-- Each line's share of the order's discount and its tax, after the order was priced again
UPDATE order_lines l
SET    discount_cents = v.discount_cents, tax_cents = v.tax_cents
FROM   (SELECT unnest(sqlc.arg(ids)::uuid[]) AS id, unnest(sqlc.arg(discounts)::bigint[]) AS discount_cents, unnest(sqlc.arg(taxes)::bigint[]) AS tax_cents) v
WHERE  l.id = v.id AND l.order_id = sqlc.arg(order_id);

-- name: UpdateOrderLineQuantity :one
-- Edit a line's quantity + recomputed line total - guarded to active lines. Component rows hold line totals, so a kit
-- line's items rescale by new/old in the same statement (a bundle line's quantity can't be edited)
WITH old AS (
    SELECT ol.id, ol.quantity FROM order_lines ol WHERE ol.id = sqlc.arg(id) AND ol.removed_at IS NULL FOR UPDATE
), scaled AS (
    UPDATE order_line_components c
    SET quantity = ROUND(c.quantity * sqlc.arg(quantity)::numeric / old.quantity, 4)
    FROM old
    WHERE c.order_line_id = old.id
)
UPDATE order_lines l
SET    quantity = sqlc.arg(quantity)::numeric, total_price = sqlc.arg(total_price)::bigint
FROM   old
WHERE  l.id = old.id
RETURNING l.*;

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
       tax_amount = $5, total_amount = $6, shipping_tax_amount = $7, fees = $8, updated_at = NOW()
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
