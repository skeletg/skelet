-- name: CreateChannel :one
INSERT INTO channels (
    slug, name, default_locale,
    currency_code,
    public_url, storefront_host, metadata
) VALUES (
    $1, $2, $3,
    $4,
    $5, $6, $7
)
RETURNING *;

-- name: GetChannelByID :one
SELECT * FROM channels WHERE id = $1;

-- name: GetChannelBySlug :one
SELECT * FROM channels WHERE slug = $1;

-- name: GetChannelByIDForUpdate :one
SELECT * FROM channels WHERE id = $1 FOR UPDATE;

-- name: GetChannelByStorefrontHost :one
-- storefront_host is normalized in Go from public_url on every write, and unique when non-empty
SELECT * FROM channels WHERE storefront_host = $1 AND storefront_host <> '';

-- name: GetDefaultChannel :one
-- The opt-in default storefront row used when a request can't be matched against X-Channel-Id or Host - returns sql.ErrNoRows when no row has is_default=TRUE - callers should treat that as "no store context"
SELECT * FROM channels WHERE is_default = TRUE LIMIT 1;

-- name: ListChannels :many
SELECT * FROM channels ORDER BY name;

-- name: ListChannelsByIDs :many
-- Batched loader for list resolvers that pre-hydrate Channel rows referenced by ProductChannelListing / CategoryChannelListing / inventory assignments - one query keyed by id replaces N GetByID round-trips
SELECT * FROM channels WHERE id = ANY(sqlc.arg(channel_ids)::uuid[]);

-- name: UpdateChannel :one
UPDATE channels
SET name            = $2,
    default_locale  = $3,
    is_active       = $4,
    currency_code   = $5,
    public_url      = $6,
    storefront_host = $7,
    metadata        = COALESCE(sqlc.narg(metadata)::jsonb, metadata)
WHERE id = $1
RETURNING *;

-- name: DeleteChannel :exec
DELETE FROM channels WHERE id = $1;

-- name: UnsetDefaultChannelExcept :many
-- Clears the is_default flag from any channel other than the given id Paired with SetChannelAsDefault inside a single transaction so the "at most one default" partial unique index never sees two TRUE rows
UPDATE channels SET is_default = FALSE
WHERE is_default = TRUE AND id <> $1
RETURNING id;

-- name: SetChannelAsDefault :one
UPDATE channels SET is_default = TRUE WHERE id = $1 RETURNING *;

-- name: CountInheritingListingsForChannel :one
-- How many listings on this channel inherit price (price IS NULL) - the currency-change guard runs it inside the channel update's transaction
SELECT COUNT(*)::bigint
FROM catalog_product_channel_listings
WHERE channel_id = $1 AND price IS NULL;

-- ==================== Channel Locales (junction) ====================

-- name: ListChannelLocales :many
SELECT l.code, l.name, l.native_name, l.is_active, l.is_rtl,
       l.sort_order AS locale_sort_order, l.created_at, l.updated_at,
       svl.sort_order AS junction_sort_order
FROM channel_locales svl
JOIN locales l ON l.code = svl.locale_code
WHERE svl.channel_id = $1
ORDER BY svl.sort_order, l.code;

-- name: ListChannelLocalesByStoreIDs :many
-- Batched loader so admin views that render N stores don't hit N queries
SELECT svl.channel_id,
       l.code, l.name, l.native_name, l.is_active, l.is_rtl,
       l.sort_order AS locale_sort_order, l.created_at, l.updated_at,
       svl.sort_order AS junction_sort_order
FROM channel_locales svl
JOIN locales l ON l.code = svl.locale_code
WHERE svl.channel_id = ANY(sqlc.arg(channel_ids)::uuid[])
ORDER BY svl.channel_id, svl.sort_order, l.code;

-- name: DeleteChannelLocales :exec
DELETE FROM channel_locales WHERE channel_id = $1;

-- name: InsertChannelLocale :exec
INSERT INTO channel_locales (channel_id, locale_code, sort_order)
VALUES ($1, LOWER($2), $3);
