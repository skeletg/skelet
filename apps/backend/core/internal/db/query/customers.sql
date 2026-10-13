-- name: CreateCustomer :one
-- customer_group_id falls back to the current default group, so a caller names one only when it
-- means to - a customer.registering plugin routing the account, say
INSERT INTO customers (email, password_hash, first_name, last_name, channel_id, is_email_verified, customer_group_id)
VALUES (sqlc.arg(email), sqlc.arg(password_hash), sqlc.arg(first_name), sqlc.arg(last_name), sqlc.arg(channel_id),
        sqlc.arg(is_email_verified),
        COALESCE(sqlc.narg(customer_group_id)::uuid, (SELECT id FROM customer_groups WHERE is_default)))
RETURNING *;

-- name: GetCustomerByChannelAndEmail :one
-- Matches the way customers_store_email_lower_key enforces uniqueness, so the lookup rides that index
SELECT * FROM customers WHERE channel_id = $1 AND lower(btrim(email)) = $2 LIMIT 1;

-- name: GetCustomerByIDForUpdate :one
-- Locks the row for a read-merge-write, so two admin edits can't each overwrite the other's fields
SELECT * FROM customers WHERE id = $1 FOR UPDATE;

-- name: GetCustomerByID :one
SELECT * FROM customers WHERE id = $1 LIMIT 1;

-- GetCustomersByIDs is the batched form of GetCustomerByID - powers the storefront DataLoader so a page that surfaces N reviews with different authors fires one query instead of N
-- name: GetCustomersByIDs :many
SELECT * FROM customers WHERE id = ANY(sqlc.arg(ids)::uuid[]);

-- name: ListCustomersInChannels :many
SELECT sqlc.embed(c), sqlc.embed(g)
FROM   customers c
JOIN   customer_groups g ON g.id = c.customer_group_id
WHERE  sqlc.arg(all_channels)::boolean OR c.channel_id = ANY(sqlc.arg(channel_ids)::uuid[])
ORDER  BY c.created_at DESC;

-- name: UpdateCustomer :execrows
UPDATE customers
SET first_name = $2, last_name = $3, phone = $4, updated_at = NOW()
WHERE id = $1;

-- name: UpdateCustomerPassword :exec
UPDATE customers SET password_hash = $2, updated_at = NOW() WHERE id = $1;

-- name: MarkCustomerEmailVerified :execrows
-- Only while the row still has the address that was proven - zero rows means it changed underneath
UPDATE customers SET is_email_verified = TRUE, updated_at = NOW()
WHERE id = sqlc.arg(id) AND email = sqlc.arg(email);

-- name: CreateCustomerAdmin :one
-- Admin-side create: accepts all admin-visible fields - caller must resolve
-- the customer_group_id (default or explicit) before invoking
INSERT INTO customers (
    email, password_hash, first_name, last_name, phone,
    is_active, is_email_verified, channel_id, customer_group_id
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: UpdateCustomerAdmin :exec
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
WHERE id = $1;

-- name: SetCustomerActive :execrows
UPDATE customers SET is_active = $2, updated_at = NOW()
WHERE id = $1;

-- name: SetDefaultBillingAddress :execrows
-- Zero rows when the address isn't this customer's - the ownership check and the write are one statement
UPDATE customers c SET default_billing_address_id = sqlc.arg(address_id), updated_at = NOW()
WHERE  c.id = sqlc.arg(customer_id)
  AND  EXISTS (SELECT 1 FROM addresses a WHERE a.id = sqlc.arg(address_id) AND a.customer_id = sqlc.arg(customer_id));

-- name: SetDefaultShippingAddress :execrows
UPDATE customers c SET default_shipping_address_id = sqlc.arg(address_id), updated_at = NOW()
WHERE  c.id = sqlc.arg(customer_id)
  AND  EXISTS (SELECT 1 FROM addresses a WHERE a.id = sqlc.arg(address_id) AND a.customer_id = sqlc.arg(customer_id));

-- name: DeleteCustomer :execrows
DELETE FROM customers WHERE id = $1;

-- name: CreateAddress :one
INSERT INTO addresses (customer_id, first_name, last_name, company, street_1, street_2, city, state, postal_code, country_code, phone)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING *;

-- name: GetAddressByID :one
-- Owner-blind, for the admin path that resolves the owner before its channel check - everything else reads through GetCustomerAddress
SELECT * FROM addresses WHERE id = $1 LIMIT 1;

-- name: GetCustomerAddress :one
SELECT * FROM addresses WHERE id = $1 AND customer_id = $2;

-- name: ListAddressesByCustomer :many
SELECT * FROM addresses WHERE customer_id = $1 ORDER BY created_at ASC;

-- name: CountAddressesByCustomer :one
SELECT COUNT(*) FROM addresses WHERE customer_id = $1;

-- name: UpdateAddress :one
UPDATE addresses
SET first_name = $3, last_name = $4, company = $5, street_1 = $6, street_2 = $7,
    city = $8, state = $9, postal_code = $10, country_code = $11, phone = $12
WHERE id = $1 AND customer_id = $2
RETURNING *;

-- name: DeleteAddress :execrows
DELETE FROM addresses WHERE id = $1 AND customer_id = $2;

-- --- customer_groups ---

-- name: CreateCustomerGroup :one
INSERT INTO customer_groups (name, slug, description) VALUES ($1, $2, $3) RETURNING *;

-- name: UpdateCustomerGroup :one
UPDATE customer_groups SET name = $2, description = $3 WHERE id = $1 RETURNING *;

-- name: DeleteCustomerGroup :exec
-- Only a non-default group goes - a group made default since it was read is left alone
DELETE FROM customer_groups WHERE id = $1 AND NOT is_default;

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

-- name: SetCustomerGroup :execrows
UPDATE customers SET customer_group_id = $2, updated_at = NOW() WHERE id = $1;

-- name: SetCustomerTaxStatus :execrows
UPDATE customers
SET tax_exempt = $2, tax_exemption_note = $3, updated_at = NOW()
WHERE id = $1;

-- name: SetCustomerVatID :execrows
UPDATE customers
SET vat_id = $2, vat_id_status = $3, vat_id_checked_at = NOW(), updated_at = NOW()
WHERE id = $1;

-- name: ListCustomersPaged :many
-- Server-side page for the admin list - ListCustomers returns the WHOLE table, which the dashboard then paged and searched in the browser - fine at seed scale, a hung tab and an ~80MB response at 200k customers Channel scoping is applied here rather than post-filtered in the resolver, or LIMIT would be taken before the permission filter and pages would come back short with holes
-- The group rides the same join the slug filter needs, and search is a pattern the caller has already escaped (the trigram indexes back it)
SELECT sqlc.embed(c), sqlc.embed(g), COUNT(*) OVER () AS total_count
FROM   customers c
JOIN   customer_groups g ON g.id = c.customer_group_id
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
