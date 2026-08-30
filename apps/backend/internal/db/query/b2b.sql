-- name: CreateCompany :one
INSERT INTO companies (channel_id, name, credit_limit_cents, payment_terms_days)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetCompany :one
SELECT * FROM companies WHERE id = $1;

-- name: GetCompanyForUpdate :one
SELECT * FROM companies WHERE id = $1 FOR UPDATE;

-- name: ListCompanies :many
SELECT * FROM companies
WHERE  (sqlc.arg(all_channels)::bool OR channel_id = ANY (sqlc.arg(channel_ids)::uuid[]))
ORDER  BY name;

-- name: UpdateCompany :one
UPDATE companies
SET    name = $2, status = $3, credit_limit_cents = $4, payment_terms_days = $5, updated_at = now()
WHERE  id = $1
RETURNING *;

-- name: AdjustCompanyCreditUsed :one
-- Signed delta: on-account placement reserves (+), recording payment
-- releases (-) - floored at zero on release
UPDATE companies
SET    credit_used_cents = GREATEST(0, credit_used_cents + $2), updated_at = now()
WHERE  id = $1
RETURNING *;

-- name: DeleteCompany :exec
DELETE FROM companies WHERE id = $1;

-- name: UpsertCompanyMember :exec
INSERT INTO company_members (company_id, customer_id, role)
VALUES ($1, $2, $3)
ON CONFLICT (customer_id) DO UPDATE SET company_id = EXCLUDED.company_id, role = EXCLUDED.role;

-- name: RemoveCompanyMember :exec
DELETE FROM company_members WHERE customer_id = $1;

-- name: ListCompanyMembers :many
SELECT cm.*, c.email, c.first_name, c.last_name
FROM   company_members cm
JOIN   customers c ON c.id = cm.customer_id
WHERE  cm.company_id = $1
ORDER  BY c.email;

-- name: GetMembershipByCustomer :one
SELECT cm.*, co.name AS company_name, co.status AS company_status,
       co.credit_limit_cents, co.credit_used_cents, co.payment_terms_days
FROM   company_members cm
JOIN   companies co ON co.id = cm.company_id
WHERE  cm.customer_id = $1;

-- name: InsertB2BQuote :one
INSERT INTO b2b_quotes (company_id, customer_id, channel_id, items, note)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetB2BQuote :one
SELECT * FROM b2b_quotes WHERE id = $1;

-- name: ListB2BQuotesForCustomer :many
SELECT * FROM b2b_quotes
WHERE  customer_id = $1
ORDER  BY created_at DESC
LIMIT  50;

-- name: ListB2BQuotesForAdmin :many
SELECT * FROM b2b_quotes
WHERE  (sqlc.arg(all_channels)::bool OR channel_id = ANY (sqlc.arg(channel_ids)::uuid[]))
  AND  (sqlc.arg(status)::text = '' OR status = sqlc.arg(status)::text)
ORDER  BY created_at DESC
LIMIT  $1 OFFSET $2;

-- name: UpdateB2BQuoteAdmin :one
-- Admin prices the quote: per-line unit prices land back in items, seller_note is the reply the buyer reads
UPDATE b2b_quotes
SET    status = $2, items = $3, total_cents = $4, seller_note = $5, valid_until = $6, updated_at = now()
WHERE  id = $1
RETURNING *;

-- name: SetB2BQuoteStatus :one
UPDATE b2b_quotes
SET    status = $2, updated_at = now()
WHERE  id = $1 AND customer_id = $3
RETURNING *;

-- name: InsertB2BCreditEntry :one
INSERT INTO b2b_credit_entries (company_id, order_id, amount_cents, reason, admin_id)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: ListB2BCreditEntries :many
SELECT * FROM b2b_credit_entries
WHERE  company_id = $1
ORDER  BY created_at DESC, id DESC
LIMIT  $2;

-- name: B2BCreditEntriesForOrder :many
SELECT * FROM b2b_credit_entries
WHERE  order_id = $1
ORDER  BY created_at;

-- name: SetCompanyDefaultGroup :one
UPDATE companies
SET    default_customer_group_id = sqlc.narg(group_id), updated_at = now()
WHERE  id = $1
RETURNING *;

-- name: GetCustomerGroupIDBySlug :one
SELECT id FROM customer_groups WHERE slug = $1;

-- name: AssignCustomerGroup :exec
-- B2B member attach moves the customer into the company's wholesale
-- group so the existing group/tier pricing engine applies
UPDATE customers SET customer_group_id = $2, updated_at = now() WHERE id = $1;

-- name: SetCompanyMemberOrderLimit :exec
UPDATE company_members SET order_limit_cents = $2 WHERE customer_id = $1;

-- name: ListOrdersByCompany :many
SELECT o.id, o.number, o.status, o.payment_status, o.total_amount,
       o.currency_code, o.created_at, cu.email AS member_email
FROM   orders o
JOIN   company_members cm ON cm.customer_id = o.customer_id
JOIN   customers cu ON cu.id = o.customer_id
WHERE  cm.company_id = $1
ORDER  BY o.created_at DESC
LIMIT  $2;

-- name: SetB2BQuoteStatusAdmin :one
UPDATE b2b_quotes
SET    status = $2, updated_at = now()
WHERE  id = $1
RETURNING *;

-- name: GetCustomerGroupSlugByID :one
SELECT slug FROM customer_groups WHERE id = $1;

-- name: UpsertCompanyPrice :one
INSERT INTO company_prices (company_id, product_id, price_cents)
VALUES ($1, $2, $3)
ON CONFLICT (company_id, product_id)
DO UPDATE SET price_cents = EXCLUDED.price_cents, updated_at = NOW()
RETURNING *;

-- name: DeleteCompanyPrice :execrows
-- Scoped by company so a price id belonging to a DIFFERENT company can't be
-- deleted by an admin who only manages this one - the resolver checks the
-- permission against company_id, so the delete has to key on it too
DELETE FROM company_prices WHERE id = $1 AND company_id = $2;

-- name: ListCompanyPrices :many
SELECT cp.*, p.sku AS product_sku
FROM company_prices cp
JOIN catalog_products p ON p.id = cp.product_id
WHERE cp.company_id = $1
ORDER BY p.sku;

-- name: ContractPriceForCustomer :one
-- The negotiated unit price a customer gets on a product through
-- their (active) company membership
SELECT cp.price_cents
FROM company_prices cp
JOIN company_members m ON m.company_id = cp.company_id
JOIN companies c ON c.id = cp.company_id AND c.status = 'active'
WHERE m.customer_id = $1 AND cp.product_id = $2;

-- name: ExpireB2BQuotes :execrows
-- Sweep quotes past their validity window into 'expired' (rather than
-- only checked lazily at accept time)
UPDATE b2b_quotes
SET status = 'expired', updated_at = NOW()
WHERE status IN ('requested', 'quoted')
  AND valid_until IS NOT NULL
  AND valid_until < NOW();

-- Invoices (on-account orders; settling releases reserved credit)

-- name: CreateB2BInvoice :one
INSERT INTO b2b_invoices (company_id, order_id, number, amount_cents, due_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetB2BInvoice :one
SELECT * FROM b2b_invoices WHERE id = $1;

-- name: ListB2BInvoicesByCompany :many
SELECT i.*, o.number AS order_number
FROM b2b_invoices i
JOIN orders o ON o.id = i.order_id
WHERE i.company_id = $1
ORDER BY i.created_at DESC
LIMIT $2;

-- SettleB2BInvoice flips an OPEN invoice to paid/void atomically - the
-- WHERE guard makes double-settlement return no row instead of
-- double-releasing credit
-- name: SettleB2BInvoice :one
UPDATE b2b_invoices
SET status = $2,
    paid_at = CASE WHEN $2 = 'paid' THEN NOW() ELSE paid_at END,
    updated_at = NOW()
WHERE id = $1 AND status = 'open'
RETURNING *;

-- name: VoidOpenB2BInvoiceForOrder :execrows
-- Closes the invoice when its order is cancelled - without this the invoice
-- stays 'open' after releaseForOrder already returned the headroom, and
-- SettleB2BInvoice's status='open' guard is still satisfied - so voiding it
-- in the admin releases the SAME amount a second time and inflates the
-- company's available credit
UPDATE b2b_invoices
SET    status = 'void', updated_at = NOW()
WHERE  order_id = $1 AND status = 'open';
