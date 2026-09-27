-- name: CreateCustomer :one
-- customer_group_id is auto-resolved to the current default group via subquery;
-- callers don't need to know which group to assign on registration
INSERT INTO customers (email, password_hash, first_name, last_name, channel_id, customer_group_id)
VALUES ($1, $2, $3, $4, $5, (SELECT id FROM customer_groups WHERE is_default))
RETURNING *;

-- name: GetCustomerByChannelAndEmail :one
SELECT * FROM customers WHERE channel_id = $1 AND email = $2 LIMIT 1;

-- name: GetCustomerByID :one
SELECT * FROM customers WHERE id = $1 LIMIT 1;

-- GetCustomersByIDs is the batched form of GetCustomerByID - powers the storefront DataLoader so a page that surfaces N reviews with different authors fires one query instead of N
-- name: GetCustomersByIDs :many
SELECT * FROM customers WHERE id = ANY(sqlc.arg(ids)::uuid[]);

-- name: ListCustomers :many
SELECT * FROM customers ORDER BY created_at DESC;

-- name: ListCustomersInChannels :many
SELECT * FROM customers
WHERE sqlc.arg(all_channels)::boolean OR channel_id = ANY(sqlc.arg(channel_ids)::uuid[])
ORDER BY created_at DESC;

-- name: UpdateCustomer :one
UPDATE customers
SET first_name = $2, last_name = $3, phone = $4, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: UpdateCustomerPassword :exec
UPDATE customers SET password_hash = $2, updated_at = NOW() WHERE id = $1;

-- name: MarkCustomerEmailVerified :one
UPDATE customers SET is_email_verified = TRUE, updated_at = NOW()
WHERE id = $1 RETURNING *;

-- name: CreateCustomerAdmin :one
-- Admin-side create: accepts all admin-visible fields - caller must resolve
-- the customer_group_id (default or explicit) before invoking
INSERT INTO customers (
    email, password_hash, first_name, last_name, phone,
    is_active, is_email_verified, channel_id, customer_group_id
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: UpdateCustomerAdmin :one
-- Admin-side update: all fields, caller pre-merges with current values
UPDATE customers
SET email                       = $2,
    first_name                  = $3,
    last_name                   = $4,
    phone                       = $5,
    is_active                   = $6,
    is_email_verified           = $7,
    note                        = $8,
    customer_group_id           = $9,
    default_billing_address_id  = $10,
    default_shipping_address_id = $11,
    metadata                    = $12,
    updated_at                  = NOW()
WHERE id = $1
RETURNING *;

-- name: SetCustomerActive :one
UPDATE customers SET is_active = $2, updated_at = NOW()
WHERE id = $1 RETURNING *;

-- name: SetDefaultBillingAddress :one
UPDATE customers SET default_billing_address_id = $2, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: SetDefaultShippingAddress :one
UPDATE customers SET default_shipping_address_id = $2, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteCustomer :exec
DELETE FROM customers WHERE id = $1;

-- name: CreateAddress :one
INSERT INTO addresses (customer_id, first_name, last_name, company, street_1, street_2, city, state, postal_code, country_code, phone)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING *;

-- name: GetAddressByID :one
SELECT * FROM addresses WHERE id = $1 LIMIT 1;

-- name: ListAddressesByCustomer :many
SELECT * FROM addresses WHERE customer_id = $1 ORDER BY created_at ASC;

-- name: UpdateAddress :one
UPDATE addresses
SET first_name = $2, last_name = $3, company = $4, street_1 = $5, street_2 = $6,
    city = $7, state = $8, postal_code = $9, country_code = $10, phone = $11
WHERE id = $1
RETURNING *;

-- name: DeleteAddress :exec
DELETE FROM addresses WHERE id = $1;

-- --- customer_groups ---

-- name: CreateCustomerGroup :one
INSERT INTO customer_groups (name, slug, description) VALUES ($1, $2, $3) RETURNING *;

-- name: UpdateCustomerGroup :one
UPDATE customer_groups SET name = $2, description = $3 WHERE id = $1 RETURNING *;

-- name: DeleteCustomerGroup :exec
DELETE FROM customer_groups WHERE id = $1;

-- name: GetCustomerGroupByID :one
SELECT * FROM customer_groups WHERE id = $1;

-- name: GetCustomerGroupBySlug :one
SELECT * FROM customer_groups WHERE slug = $1;

-- name: GetDefaultCustomerGroup :one
SELECT * FROM customer_groups WHERE is_default = TRUE LIMIT 1;

-- name: ListCustomerGroupsWithMemberCount :many
SELECT g.id, g.name, g.slug, g.description, g.is_default, g.created_at,
       COALESCE(c.member_count, 0)::bigint AS member_count
FROM customer_groups g
LEFT JOIN (
    SELECT customer_group_id, COUNT(*) AS member_count
    FROM customers
    GROUP BY customer_group_id
) c ON c.customer_group_id = g.id
ORDER BY g.name;

-- name: CountCustomerGroupMembers :one
SELECT COUNT(*) FROM customers WHERE customer_group_id = $1;

-- name: ClearDefaultCustomerGroup :exec
UPDATE customer_groups SET is_default = FALSE WHERE is_default = TRUE;

-- name: MarkCustomerGroupDefault :exec
UPDATE customer_groups SET is_default = TRUE WHERE id = $1;

-- name: SetCustomerGroup :one
UPDATE customers SET customer_group_id = $2, updated_at = NOW() WHERE id = $1 RETURNING *;

-- name: SetCustomerTaxStatus :one
UPDATE customers
SET tax_exempt = $2, tax_exemption_note = $3, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: SetCustomerVatID :one
UPDATE customers
SET vat_id = $2, vat_id_status = $3, vat_id_checked_at = NOW(), updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: ListCustomersPaged :many
-- Server-side page for the admin list - ListCustomers returns the WHOLE table, which the dashboard then paged and searched in the browser - fine at seed scale, a hung tab and an ~80MB response at 200k customers Channel scoping is applied here rather than post-filtered in the resolver, or LIMIT would be taken before the permission filter and pages would come back short with holes
SELECT c.*, COUNT(*) OVER () AS total_count
FROM   customers c
LEFT   JOIN customer_groups g ON g.id = c.customer_group_id
WHERE  (sqlc.arg(all_channels)::boolean OR c.channel_id = ANY(sqlc.arg(channel_ids)::uuid[]))
  AND  (sqlc.arg(search)::text = '' OR
        c.email ILIKE '%' || sqlc.arg(search)::text || '%' OR
        c.first_name ILIKE '%' || sqlc.arg(search)::text || '%' OR
        c.last_name ILIKE '%' || sqlc.arg(search)::text || '%')
  AND  (sqlc.arg(group_slug)::text = '' OR g.slug = sqlc.arg(group_slug)::text)
  AND  (NOT sqlc.arg(filter_active)::boolean OR c.is_active = sqlc.arg(is_active)::boolean)
ORDER  BY c.created_at DESC
LIMIT  sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: IsCustomerActive :one
-- Backs the per-request session guard, so keep it to the single boolean - the guard runs on every authenticated storefront request and doesn't need the full row GetCustomerByID returns
SELECT is_active FROM customers WHERE id = $1;

-- name: CustomerEmailsByIDs :many
-- Addresses for a known set of ids, skipping any that no longer exist - the commerce half of the audience export, whose membership comes from the signals database
SELECT email FROM customers WHERE id = ANY(sqlc.arg(customer_ids)::uuid[]) ORDER BY email;

-- name: CustomerEmailsByIDsInChannel :many
SELECT email FROM customers WHERE id = ANY(sqlc.arg(customer_ids)::uuid[]) AND channel_id = sqlc.arg(channel_id) ORDER BY email;

-- name: CommerceClock :one
-- The commerce database's clock, the one every roster read_at is stamped on
SELECT statement_timestamp()::timestamptz AS now;
