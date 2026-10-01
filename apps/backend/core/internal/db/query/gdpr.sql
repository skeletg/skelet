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
    business_ids        = '[]'::jsonb,
    metadata            = '{}'::jsonb,
    billing_first_name  = 'Redacted', billing_last_name = '',
    billing_company     = '', billing_street_1 = 'Redacted', billing_street_2 = '',
    billing_city        = 'Redacted', billing_state = '', billing_postal_code = '',
    billing_phone       = '',
    shipping_first_name = 'Redacted', shipping_last_name = '',
    shipping_company    = '', shipping_street_1 = 'Redacted', shipping_street_2 = '',
    shipping_city       = 'Redacted', shipping_state = '', shipping_postal_code = '',
    shipping_phone      = '',
    anon_id             = ''
WHERE customer_id = $1;

-- name: DeleteCustomerWishlist :exec
DELETE FROM wishlist_items WHERE customer_id = $1;

-- name: ScrubCustomerCarts :execrows
-- carts.email makes an abandoned cart addressable by the recovery flow, so an
-- un-scrubbed row keeps emailing a customer who asked to be erased - and a
-- checkout in progress holds addresses, phone and tax details the same way
UPDATE carts
SET    email = '', shipping_address = NULL, billing_address = NULL,
       contact_phone = '', company_name = '', business_ids = '{}'::jsonb, po_number = '',
       pickup_contact = NULL
WHERE  customer_id = $1
  AND  (email <> '' OR shipping_address IS NOT NULL OR billing_address IS NOT NULL
        OR contact_phone <> '' OR company_name <> '' OR business_ids <> '{}'::jsonb OR po_number <> ''
        OR pickup_contact IS NOT NULL);

-- name: DeleteCustomerBackInStockRequests :execrows
-- Keyed by email as well as customer_id (guests subscribe too), and only on the customer's channel - the same
-- address on another store belongs to a separate account
DELETE FROM back_in_stock_requests
WHERE  channel_id = sqlc.arg(channel_id)
  AND  (customer_id = sqlc.arg(customer_id) OR lower(btrim(email)) = sqlc.arg(email)::text);

-- name: ScrubCustomerAuditDetail :execrows
-- The audit log keeps admin mutation variables for a year, so an adminUpdateCustomer or adminAddAddress row still holds the pre-erasure email, phone and street long after the customer row was scrubbed - only the detail payload is cleared, the action, actor and timestamp stay
-- A row names the customer by address, by id, or through one of their orders; position() rather than ILIKE, so a % or _ in the address can't widen the match
-- Runs before the orders are scrubbed, while the guest orders still carry the address
UPDATE audit_log a
SET    detail = jsonb_build_object('redacted', true, 'reason', 'customer erased')
WHERE  position(sqlc.arg(email)::text IN lower(a.detail::text)) > 0
   OR  position(sqlc.arg(customer_id)::uuid::text IN a.detail::text) > 0
   OR  EXISTS (
         SELECT 1 FROM orders o
         WHERE  (o.customer_id = sqlc.arg(customer_id)
                 OR (o.customer_id IS NULL AND o.channel_id = sqlc.arg(channel_id) AND lower(btrim(o.email)) = sqlc.arg(email)::text))
           AND  position(o.id::text IN a.detail::text) > 0);

-- name: ScrubGuestOrdersByEmail :execrows
-- Orders placed as a guest under the erased address, on the customer's channel - retained like their own orders, with the same columns scrubbed
UPDATE orders
SET email               = 'redacted-' || sqlc.arg(customer_id)::uuid::text || '@anonymized.invalid',
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
    shipping_phone      = '',
    anon_id             = ''
WHERE customer_id IS NULL
  AND channel_id = sqlc.arg(channel_id)
  AND lower(btrim(email)) = sqlc.arg(email)::text;

-- name: ScrubCustomerProductQuestions :execrows
-- The question text stays on the product page, the asker's name and address don't
UPDATE product_questions
SET    author_name = 'Redacted', author_email = ''
WHERE  customer_id = sqlc.arg(customer_id)
   OR  (channel_id = sqlc.arg(channel_id) AND lower(btrim(author_email)) = sqlc.arg(email)::text);

-- name: DeleteCustomerSignupSubmissions :execrows
DELETE FROM signup_form_submissions
WHERE  customer_id = sqlc.arg(customer_id)
   OR  (channel_id = sqlc.arg(channel_id) AND lower(btrim(email)) = sqlc.arg(email)::text);

-- name: DeleteCustomerWebFormSubmissions :execrows
-- The answers are free-form, so a submission naming the address anywhere in them goes, not just one tied to the account
DELETE FROM web_form_submissions
WHERE  customer_id = sqlc.arg(customer_id)
   OR  (channel_id = sqlc.arg(channel_id) AND position(sqlc.arg(email)::text IN lower(answers::text)) > 0);

-- name: ScrubCustomerDownloadGrants :execrows
-- Grants outlive the order they came from, so the address they were issued to is blanked here
UPDATE download_grants g
SET    email = ''
WHERE  g.customer_id = sqlc.arg(customer_id)
   OR  (lower(btrim(g.email)) = sqlc.arg(email)::text
        AND EXISTS (SELECT 1 FROM orders o WHERE o.id = g.order_id AND o.channel_id = sqlc.arg(channel_id)));

-- name: ScrubCustomerGiftCards :execrows
-- A card sent to the erased address loses the address and the sender's note; one the customer bought loses the note they wrote
UPDATE gift_cards
SET    recipient_email = CASE WHEN lower(btrim(recipient_email)) = sqlc.arg(email)::text THEN '' ELSE recipient_email END,
       message         = '',
       updated_at      = NOW()
WHERE  customer_id = sqlc.arg(customer_id)
   OR  (channel_id = sqlc.arg(channel_id) AND lower(btrim(recipient_email)) = sqlc.arg(email)::text);

-- name: DeleteCustomerPaymentMethods :execrows
DELETE FROM customer_payment_methods WHERE customer_id = $1;

-- name: DeleteCustomerWishlistShares :execrows
DELETE FROM wishlist_shares WHERE customer_id = $1;
