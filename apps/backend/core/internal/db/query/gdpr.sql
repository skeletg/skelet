-- GDPR anonymize-in-place - orders are retained (tax-law basis) with their PII columns scrubbed; the customer row keeps
-- its id (FK anchor) with all personal fields redacted

-- name: AnonymizeCustomerRow :exec
UPDATE customers
SET email             = 'redacted-' || id::text || '@anonymized.invalid',
    first_name        = 'Redacted',
    last_name         = '',
    phone             = NULL,
    note              = '',
    metadata          = '{}'::jsonb,
    is_active         = FALSE,
    is_email_verified = FALSE,
    password_hash     = $2,
    updated_at        = NOW()
WHERE id = $1;

-- name: DeleteCustomerAddresses :exec
DELETE FROM addresses WHERE customer_id = $1;

-- name: ScrubCustomerOrders :execrows
-- customer_note, po_number and metadata are free text the shopper typed - "leave with Jane at #42, ring me on +44 7700 900123" is exactly the kind of content that survived an erasure while the structured address columns next to it read "Redacted"
UPDATE orders
SET email               = 'redacted-' || customer_id::text || '@anonymized.invalid',
    customer_note       = '',
    po_number           = NULL,
    metadata            = '{}'::jsonb,
    billing_first_name  = 'Redacted', billing_last_name = '',
    billing_company     = '', billing_street_1 = 'Redacted', billing_street_2 = '',
    billing_city        = 'Redacted', billing_state = '', billing_postal_code = '',
    billing_phone       = '',
    shipping_first_name = 'Redacted', shipping_last_name = '',
    shipping_company    = '', shipping_street_1 = 'Redacted', shipping_street_2 = '',
    shipping_city       = 'Redacted', shipping_state = '', shipping_postal_code = '',
    shipping_phone      = ''
WHERE customer_id = $1;

-- name: DeleteCustomerWishlist :exec
DELETE FROM wishlist_items WHERE customer_id = $1;

-- name: ScrubCustomerCarts :execrows
-- carts.email makes an abandoned cart addressable by the recovery flow, so an
-- un-scrubbed row keeps emailing a customer who asked to be erased
UPDATE carts
SET    email = ''
WHERE  customer_id = $1 AND email <> '';

-- name: DeleteCustomerBackInStockRequests :execrows
-- Keyed by email rather than customer_id (guests subscribe too), so this takes
-- the address the customer is being erased under
DELETE FROM back_in_stock_requests WHERE lower(email) = lower(sqlc.arg(email)::text);

-- name: ScrubCustomerAuditDetail :execrows
-- The audit log keeps admin mutation variables for a year, so an adminUpdateCustomer or adminAddAddress row still holds the pre-erasure email, phone and street long after the customer row was scrubbed - only the detail payload is cleared - the action, actor and timestamp stay, so the trail of WHO did WHAT survives the erasure
UPDATE audit_log
SET    detail = jsonb_build_object('redacted', true, 'reason', 'customer erased')
WHERE  detail::text ILIKE '%' || sqlc.arg(email)::text || '%';
