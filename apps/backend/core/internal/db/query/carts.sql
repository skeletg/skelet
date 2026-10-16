-- name: GetCartByCustomerAndChannel :one
SELECT * FROM carts
WHERE customer_id = $1 AND channel_id = $2 AND status = 'active' AND kind = 'shopper'
LIMIT 1;

-- name: CreateCart :one
INSERT INTO carts (customer_id, channel_id) VALUES ($1, $2) RETURNING *;

-- name: CreateOfferCart :one
-- A guest offer carries its recipient in email, a customer offer its owner in customer_id - either way it never takes the shopper-cart slot
INSERT INTO carts (customer_id, channel_id, kind, email, offer_expires_at, offer_created_by, offer_link_token)
VALUES (sqlc.narg(customer_id), sqlc.arg(channel_id), 'offer', sqlc.arg(email), sqlc.arg(expires_at), sqlc.narg(created_by), translate(rtrim(encode(public.gen_random_bytes(16), 'base64'), '='), '+/', '-_'))
RETURNING *;

-- name: SetCartStatus :one
UPDATE carts SET status = $2, updated_at = NOW()
WHERE id = $1 AND status = 'active'
RETURNING *;

-- name: GetCartByID :one
SELECT * FROM carts WHERE id = $1 LIMIT 1;

-- name: GetCartByOfferToken :one
SELECT * FROM carts WHERE kind = 'offer' AND offer_link_token = $1 LIMIT 1;

-- name: ListCarts :many
-- Admin cart console listing: channel-scoped, with optional customer / status / abandoned (updated_before) filters and a free-text search over the linked customer's email + name. The page is cut first and only its carts' lines are summed - subtotal is an estimate (fractional-cent rounding, no tax/shipping)
SELECT
    p.id                                              AS id,
    p.customer_id                                     AS customer_id,
    p.channel_id                                      AS channel_id,
    p.status                                          AS status,
    p.kind                                            AS kind,
    p.email                                           AS email,
    p.offer_expires_at                                AS offer_expires_at,
    p.offer_sent_at                                   AS offer_sent_at,
    p.offer_opened_at                                 AS offer_opened_at,
    p.offer_revoked_at                                AS offer_revoked_at,
    p.order_id                                        AS order_id,
    p.created_at                                      AS created_at,
    p.updated_at                                      AS updated_at,
    p.customer_email                                  AS customer_email,
    p.customer_first_name                             AS customer_first_name,
    p.customer_last_name                              AS customer_last_name,
    li.item_count::int                                AS item_count,
    li.subtotal::bigint                               AS subtotal
FROM (
    SELECT c.id, c.customer_id, c.channel_id, c.status, c.kind, c.email, c.offer_expires_at, c.offer_sent_at,
           c.offer_opened_at, c.offer_revoked_at, c.order_id, c.created_at, c.updated_at,
           cu.email AS customer_email, cu.first_name AS customer_first_name, cu.last_name AS customer_last_name
    FROM carts c
    LEFT JOIN customers cu ON cu.id = c.customer_id
    WHERE c.channel_id = sqlc.arg(channel_id)
      AND (sqlc.narg(customer_id)::uuid IS NULL OR c.customer_id = sqlc.narg(customer_id)::uuid)
      AND (sqlc.narg(status)::text IS NULL OR c.status = sqlc.narg(status)::text)
      AND (sqlc.narg(kind)::text IS NULL OR c.kind = sqlc.narg(kind)::text)
      AND (sqlc.narg(updated_before)::timestamptz IS NULL OR c.updated_at < sqlc.narg(updated_before)::timestamptz)
      AND (
        sqlc.narg(search)::text IS NULL
        OR cu.email ILIKE '%' || escape_like(sqlc.narg(search)::text) || '%'
        OR c.email ILIKE '%' || escape_like(sqlc.narg(search)::text) || '%'
        OR (COALESCE(cu.first_name, '') || ' ' || COALESCE(cu.last_name, '')) ILIKE '%' || escape_like(sqlc.narg(search)::text) || '%'
      )
    ORDER BY c.updated_at DESC
    LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off)
) p
CROSS JOIN LATERAL (
    SELECT COUNT(*)::int                                              AS item_count,
           COALESCE(ROUND(SUM(l.quantity * l.unit_price)), 0)::bigint AS subtotal
    FROM cart_lines l
    WHERE l.cart_id = p.id
) li
ORDER BY p.updated_at DESC;

-- name: CountCarts :one
SELECT COUNT(*)::int
FROM carts c
LEFT JOIN customers cu ON cu.id = c.customer_id
WHERE c.channel_id = sqlc.arg(channel_id)
  AND (sqlc.narg(customer_id)::uuid IS NULL OR c.customer_id = sqlc.narg(customer_id)::uuid)
  AND (sqlc.narg(status)::text IS NULL OR c.status = sqlc.narg(status)::text)
  AND (sqlc.narg(kind)::text IS NULL OR c.kind = sqlc.narg(kind)::text)
  AND (sqlc.narg(updated_before)::timestamptz IS NULL OR c.updated_at < sqlc.narg(updated_before)::timestamptz)
  AND (
    sqlc.narg(search)::text IS NULL
    OR cu.email ILIKE '%' || escape_like(sqlc.narg(search)::text) || '%'
    OR c.email ILIKE '%' || escape_like(sqlc.narg(search)::text) || '%'
    OR (COALESCE(cu.first_name, '') || ' ' || COALESCE(cu.last_name, '')) ILIKE '%' || escape_like(sqlc.narg(search)::text) || '%'
  );

-- name: UpsertCartLineBySku :one
INSERT INTO cart_lines (cart_id, product_name, product_sku, unit_price, quantity)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (cart_id, product_sku) DO UPDATE
  SET quantity   = cart_lines.quantity + EXCLUDED.quantity,
      unit_price = CASE WHEN cart_lines.price_override THEN cart_lines.unit_price ELSE EXCLUDED.unit_price END, -- an admin-set price is the offer, more of the item doesn't undo it
      updated_at = NOW()
RETURNING *;

-- name: ReassignCartToCustomer :one
UPDATE carts SET customer_id = $2, updated_at = NOW()
WHERE id = $1 AND customer_id IS NULL AND status = 'active' AND kind = 'shopper'
RETURNING *;

-- name: GetCartLineByID :one
SELECT * FROM cart_lines WHERE id = $1 LIMIT 1;

-- name: ListCartLinesByCart :many
SELECT * FROM cart_lines WHERE cart_id = $1 ORDER BY created_at ASC;

-- name: UpdateCartLineQuantity :one
UPDATE cart_lines SET quantity = $2, updated_at = NOW() WHERE id = $1 RETURNING *;

-- name: UpdateCartLineQuantityPrice :one
-- Quantity change that also re-prices the line (tier-price boundary cross)
UPDATE cart_lines SET quantity = $2, unit_price = $3, updated_at = NOW() WHERE id = $1 RETURNING *;

-- name: DeleteCartLine :exec
DELETE FROM cart_lines WHERE id = $1;

-- name: AbandonedCarts :many
-- Oldest first, so a backlog drains in order
SELECT c.id, c.channel_id, c.customer_id, c.email, c.last_activity_at AS last_activity
FROM carts c
WHERE c.channel_id = sqlc.arg(channel_id)
  AND c.status = 'active' AND c.kind = 'shopper' AND (c.customer_id IS NOT NULL OR c.email <> '')
  AND c.last_activity_at < sqlc.arg(threshold)::timestamptz
  AND c.last_activity_at > sqlc.arg(max_age)::timestamptz
  AND (c.abandoned_notified_at IS NULL OR c.abandoned_notified_at < c.last_activity_at)
  AND EXISTS (SELECT 1 FROM cart_lines l WHERE l.cart_id = c.id)
ORDER BY c.last_activity_at, c.id
LIMIT sqlc.arg(lim);

-- name: MarkCartsAbandonedNotified :exec
UPDATE carts SET abandoned_notified_at = NOW() WHERE id = ANY(sqlc.arg(ids)::uuid[]);

-- name: DeleteCartLinesByCart :exec
DELETE FROM cart_lines WHERE cart_id = $1;

-- name: SetCartEmail :exec
-- Records the email a guest enters at checkout entry so an abandoned guest cart becomes recoverable - bumps updated_at (email entry is real activity, so it is the correct start of the abandonment clock)
-- The details step writes the email with the rest of the checkout, so this moves checkout_version too
UPDATE carts SET email = $2, checkout_version = checkout_version + 1, updated_at = NOW() WHERE id = $1;

-- name: PruneStaleCarts :execrows
-- Deletes abandoned carts past a retention window: unconverted carts (active or merged) whose last activity (cart or newest line) is older than the cutoff, plus converted carts older than the cutoff (their order carries the durable record) - cart lines / applied codes cascade on delete - guards the reaper from ever touching a recently-active cart
-- An open offer waits for its recipient however long it sits untouched - it goes only once it is finished, or expired or revoked before the cutoff
DELETE FROM carts c
WHERE c.updated_at < sqlc.arg(cutoff)::timestamptz
  AND (c.kind = 'shopper' OR c.status <> 'active'
       OR c.offer_expires_at < sqlc.arg(cutoff)::timestamptz
       OR c.offer_revoked_at < sqlc.arg(cutoff)::timestamptz)
  AND COALESCE((SELECT MAX(l.updated_at) FROM cart_lines l WHERE l.cart_id = c.id), c.updated_at) < sqlc.arg(cutoff)::timestamptz;

-- Save-for-later (cart-scoped, guest-capable)

-- name: SaveCartItem :one
-- Parks a product off the cart - on conflict (same sku already saved) the
-- quantity is replaced, not summed - saving a line twice isn't "add more"
INSERT INTO cart_saved_items (cart_id, product_sku, product_name, quantity)
VALUES ($1, $2, $3, $4)
ON CONFLICT (cart_id, product_sku) DO UPDATE SET quantity = EXCLUDED.quantity, product_name = EXCLUDED.product_name
RETURNING *;

-- name: GetSavedItem :one
SELECT * FROM cart_saved_items WHERE id = $1;

-- name: DeleteSavedItem :execrows
DELETE FROM cart_saved_items WHERE id = $1 AND cart_id = $2;

-- name: ListSavedItemsByCart :many
SELECT * FROM cart_saved_items WHERE cart_id = $1 ORDER BY created_at DESC;

-- name: SetCartNote :exec
UPDATE carts SET note = $2, updated_at = NOW() WHERE id = $1;

-- name: LockCart :one
-- Row lock for a checkout step's short commit - the step was computed from an unlocked read, and checkout_version says whether the row moved since
SELECT * FROM carts WHERE id = $1 FOR UPDATE;

-- name: SetCartCheckout :exec
UPDATE carts SET
    shipping_address         = sqlc.narg(shipping_address),
    shipping_address_id      = sqlc.narg(shipping_address_id),
    billing_same_as_shipping = sqlc.arg(billing_same_as_shipping),
    billing_address          = sqlc.narg(billing_address),
    billing_address_id       = sqlc.narg(billing_address_id),
    shipping_carrier_slug    = sqlc.arg(shipping_carrier_slug),
    shipping_method_code     = sqlc.arg(shipping_method_code),
    payment_provider_slug    = sqlc.arg(payment_provider_slug),
    po_number                = sqlc.arg(po_number),
    use_store_credit         = sqlc.arg(use_store_credit),
    gift_card_codes          = sqlc.arg(gift_card_codes)::text[],
    destination_country      = sqlc.arg(destination_country),
    destination_region       = sqlc.arg(destination_region),
    destination_postal_code  = sqlc.arg(destination_postal_code),
    contact_phone            = sqlc.arg(contact_phone),
    purchase_type            = sqlc.arg(purchase_type),
    company_name             = sqlc.arg(company_name),
    business_ids             = sqlc.arg(business_ids),
    pickup_contact           = sqlc.narg(pickup_contact),
    deliveries               = sqlc.arg(deliveries),
    split_delivery           = sqlc.arg(split_delivery),
    checkout_version         = checkout_version + 1,
    updated_at               = NOW()
WHERE id = sqlc.arg(id);

-- Offers

-- name: SetCartOffer :exec
UPDATE carts SET
    offer_expires_at      = sqlc.arg(expires_at),
    offer_message         = sqlc.arg(message),
    offer_show_in_account = sqlc.arg(show_in_account),
    offer_locks           = sqlc.arg(locks)::text[],
    offer_payment_methods = sqlc.arg(payment_methods)::text[],
    updated_at            = NOW()
WHERE id = sqlc.arg(id) AND kind = 'offer';

-- name: MarkCartOfferSent :exec
UPDATE carts SET offer_sent_at = NOW(), offer_send_count = offer_send_count + 1 WHERE id = $1 AND kind = 'offer';

-- name: MarkCartOfferOpened :execrows
-- The first open only - the console shows when the recipient first looked
UPDATE carts SET offer_opened_at = NOW() WHERE id = $1 AND kind = 'offer' AND offer_opened_at IS NULL
  AND status = 'active' AND offer_revoked_at IS NULL AND offer_expires_at > clock_timestamp();

-- name: RevokeCartOffer :exec
UPDATE carts SET offer_revoked_at = NOW(), updated_at = NOW() WHERE id = $1 AND kind = 'offer' AND offer_revoked_at IS NULL;

-- name: BumpCartOfferLink :one
UPDATE carts SET offer_link_version = offer_link_version + 1,
    offer_link_token = translate(rtrim(encode(public.gen_random_bytes(16), 'base64'), '='), '+/', '-_'), updated_at = NOW() WHERE id = $1 AND kind = 'offer'
RETURNING offer_link_version;

-- name: SetCartOrder :exec
UPDATE carts SET order_id = $2 WHERE id = $1;

-- name: SetCartLinePrice :one
UPDATE cart_lines SET unit_price = $2, price_override = $3, updated_at = NOW() WHERE id = $1 RETURNING *;

-- name: InsertCustomCartLine :one
INSERT INTO cart_lines (cart_id, product_name, product_sku, unit_price, quantity, price_override, is_custom, tax_class)
VALUES (sqlc.arg(cart_id), sqlc.arg(product_name), sqlc.arg(product_sku), sqlc.arg(unit_price), sqlc.arg(quantity), true, true, sqlc.arg(tax_class))
RETURNING *;

-- name: ListCustomerCartOffers :many
-- The offers a customer's account lists - only the ones the admin chose to show, and only while they can still be completed
SELECT * FROM carts
WHERE customer_id = $1 AND channel_id = $2 AND kind = 'offer' AND status = 'active'
  AND offer_show_in_account AND offer_revoked_at IS NULL AND offer_expires_at > NOW()
ORDER BY offer_expires_at ASC;

-- name: ExpiredCartOffers :many
-- Offers that ran out unfinished and haven't been announced yet - oldest first, so a backlog drains in order
SELECT * FROM carts
WHERE kind = 'offer' AND status = 'active' AND offer_revoked_at IS NULL
  AND offer_expires_at <= NOW() AND offer_expired_notified_at IS NULL
ORDER BY offer_expires_at
LIMIT sqlc.arg(lim);

-- name: MarkCartOfferExpiredNotified :many
UPDATE carts SET offer_expired_notified_at = NOW() WHERE id = ANY(sqlc.arg(ids)::uuid[])
  AND kind = 'offer' AND status = 'active' AND offer_revoked_at IS NULL
  AND offer_expires_at <= clock_timestamp() AND offer_expired_notified_at IS NULL
RETURNING *;
