-- A data subject's rows outside the customer domain, for the access-request export - each section is one JSON array
-- of the whole set, never a page, since a bundle that silently stops at N rows answers the request wrongly
-- "Theirs" is the customer id, or the address on the customer's channel for rows guests can create too - an empty
-- address matches nothing, which is how the builder leaves out rows it can't tie to a verified owner

-- name: DSARStoreCreditEntries :one
SELECT COALESCE(jsonb_agg(jsonb_build_object(
         'id', id, 'orderId', order_id, 'amountCents', amount_cents, 'currency', currency_code,
         'reason', reason, 'createdAt', created_at) ORDER BY created_at, id), '[]')::jsonb AS doc
FROM   store_credit_entries
WHERE  customer_id = $1;

-- name: DSARLoyaltyEntries :one
SELECT COALESCE(jsonb_agg(jsonb_build_object(
         'id', id, 'orderId', order_id, 'points', points, 'reason', reason, 'createdAt', created_at)
         ORDER BY created_at, id), '[]')::jsonb AS doc
FROM   loyalty_point_entries
WHERE  customer_id = $1;

-- name: DSARReviews :one
SELECT COALESCE(jsonb_agg(jsonb_build_object(
         'id', id, 'productId', product_id, 'locale', locale, 'title', title, 'body', body, 'status', status,
         'guestName', guest_name, 'guestEmail', guest_email, 'merchantReply', merchant_reply, 'submittedAt', submitted_at)
         ORDER BY submitted_at, id), '[]')::jsonb AS doc
FROM   reviews
WHERE  customer_id = sqlc.arg(customer_id)
   OR  (sqlc.arg(email)::text <> '' AND channel_id = sqlc.arg(channel_id) AND lower(btrim(guest_email)) = sqlc.arg(email)::text);

-- name: DSARProductQuestions :one
SELECT COALESCE(jsonb_agg(jsonb_build_object(
         'id', id, 'productId', product_id, 'authorName', author_name, 'authorEmail', author_email,
         'question', question, 'answer', answer, 'status', status, 'createdAt', created_at)
         ORDER BY created_at, id), '[]')::jsonb AS doc
FROM   product_questions
WHERE  customer_id = sqlc.arg(customer_id)
   OR  (sqlc.arg(email)::text <> '' AND channel_id = sqlc.arg(channel_id) AND lower(btrim(author_email)) = sqlc.arg(email)::text);

-- name: DSARSubscriptions :one
SELECT COALESCE(jsonb_agg(jsonb_build_object(
         'id', id, 'productId', product_id, 'orderId', order_id, 'status', status, 'sku', sku, 'title', title,
         'intervalUnit', interval_unit, 'intervalCount', interval_count, 'price', price, 'currency', currency_code,
         'quantity', quantity, 'currentPeriodStart', current_period_start, 'currentPeriodEnd', current_period_end,
         'nextBillingAt', next_billing_at, 'canceledAt', canceled_at, 'createdAt', created_at)
         ORDER BY created_at, id), '[]')::jsonb AS doc
FROM   subscriptions
WHERE  customer_id = $1;

-- name: DSARGiftCards :one
-- Bought by the customer or sent to their address - the code is a bearer credential, so only its tail leaves
SELECT COALESCE(jsonb_agg(jsonb_build_object(
         'id', id, 'codeLast4', right(code, 4), 'currency', currency_code, 'initialBalance', initial_balance,
         'balance', balance, 'status', status, 'recipientEmail', recipient_email, 'message', message,
         'purchased', customer_id IS NOT DISTINCT FROM sqlc.arg(customer_id)::uuid,
         'expiresAt', expires_at, 'createdAt', created_at) ORDER BY created_at, id), '[]')::jsonb AS doc
FROM   gift_cards
WHERE  customer_id = sqlc.arg(customer_id)::uuid
   OR  (sqlc.arg(email)::text <> '' AND channel_id = sqlc.arg(channel_id) AND lower(btrim(recipient_email)) = sqlc.arg(email)::text);

-- name: DSARPaymentMethods :one
-- Display fields only - the provider token never leaves
SELECT COALESCE(jsonb_agg(jsonb_build_object(
         'id', id, 'provider', provider_slug, 'brand', brand, 'last4', last4,
         'expMonth', exp_month, 'expYear', exp_year, 'createdAt', created_at) ORDER BY created_at, id), '[]')::jsonb AS doc
FROM   customer_payment_methods
WHERE  customer_id = $1;

-- name: DSARCarts :one
SELECT COALESCE(jsonb_agg(jsonb_build_object(
         'id', c.id, 'status', c.status, 'email', c.email, 'note', c.note, 'createdAt', c.created_at,
         'lastActivityAt', c.last_activity_at,
         'lines', (SELECT COALESCE(jsonb_agg(jsonb_build_object(
                     'sku', l.product_sku, 'name', l.product_name, 'quantity', l.quantity, 'unitPrice', l.unit_price)
                     ORDER BY l.created_at, l.id), '[]')
                   FROM cart_lines l WHERE l.cart_id = c.id))
         ORDER BY c.created_at, c.id), '[]')::jsonb AS doc
FROM   carts c
WHERE  c.customer_id = $1;

-- name: DSAROrderShipments :one
-- The packages of the customer's orders, with the address each went to
SELECT COALESCE(jsonb_agg(jsonb_build_object(
         'orderId', s.order_id, 'deliveryKind', s.delivery_kind, 'deliveryOption', s.delivery_option_name,
         'method', s.method_name, 'shipTo', s.ship_to, 'point', s.point, 'createdAt', s.created_at)
         ORDER BY s.created_at, s.order_id, s.position), '[]')::jsonb AS doc
FROM   order_shipments s
JOIN   orders o ON o.id = s.order_id
WHERE  o.customer_id = $1;

-- name: DSARDownloadGrants :one
-- The token and the file URL are bearer credentials, so the grant is described rather than handed over
SELECT COALESCE(jsonb_agg(jsonb_build_object(
         'id', g.id, 'orderId', g.order_id, 'productId', g.product_id, 'title', g.title, 'email', g.email,
         'downloadsUsed', g.downloads_used, 'maxDownloads', g.max_downloads, 'expiresAt', g.expires_at,
         'createdAt', g.created_at) ORDER BY g.created_at, g.id), '[]')::jsonb AS doc
FROM   download_grants g
WHERE  g.customer_id = sqlc.arg(customer_id)
   OR  (sqlc.arg(email)::text <> '' AND lower(btrim(g.email)) = sqlc.arg(email)::text
        AND EXISTS (SELECT 1 FROM orders o WHERE o.id = g.order_id AND o.channel_id = sqlc.arg(channel_id)));

-- name: DSARBackInStockRequests :one
SELECT COALESCE(jsonb_agg(jsonb_build_object(
         'id', id, 'productId', product_id, 'sku', sku, 'email', email, 'notifiedAt', notified_at,
         'createdAt', created_at) ORDER BY created_at, id), '[]')::jsonb AS doc
FROM   back_in_stock_requests
WHERE  channel_id = sqlc.arg(channel_id)
  AND  (customer_id = sqlc.arg(customer_id) OR (sqlc.arg(email)::text <> '' AND lower(btrim(email)) = sqlc.arg(email)::text));

-- name: DSARSignupSubmissions :one
SELECT COALESCE(jsonb_agg(jsonb_build_object(
         'id', s.id, 'form', f.slug, 'email', s.email, 'createdAt', s.created_at) ORDER BY s.created_at, s.id), '[]')::jsonb AS doc
FROM   signup_form_submissions s
JOIN   signup_forms f ON f.id = s.form_id
WHERE  s.customer_id = sqlc.arg(customer_id)
   OR  (sqlc.arg(email)::text <> '' AND s.channel_id = sqlc.arg(channel_id) AND lower(btrim(s.email)) = sqlc.arg(email)::text);

-- name: DSARWebFormSubmissions :one
SELECT COALESCE(jsonb_agg(jsonb_build_object(
         'id', s.id, 'form', f.slug, 'answers', s.answers, 'createdAt', s.created_at) ORDER BY s.created_at, s.id), '[]')::jsonb AS doc
FROM   web_form_submissions s
JOIN   web_forms f ON f.id = s.form_id
WHERE  s.customer_id = sqlc.arg(customer_id)
   OR  (sqlc.arg(email)::text <> '' AND s.channel_id = sqlc.arg(channel_id) AND jsonb_typeof(s.answers) = 'object'
        AND EXISTS (SELECT 1 FROM jsonb_each_text(s.answers) a WHERE lower(btrim(a.value)) = sqlc.arg(email)::text));
