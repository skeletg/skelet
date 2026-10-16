-- ==================== Inventory Locations ====================

-- name: CreateInventoryLocation :one
INSERT INTO inventory_locations (
    slug, name, is_active,
    line1, line2, city, region, postal_code, country_code,
    latitude, longitude,
    type, timezone, hours,
    email, phone, contact_name, external_reference,
    metadata, storefront_visible, pickup_enabled, pickup_instructions
)
VALUES (
    $1, $2, $3,
    $4, $5, $6, $7, $8, $9,
    $10, $11,
    $12, $13, $14,
    $15, $16, $17, $18,
    $19, $20, $21, $22
)
RETURNING *;

-- name: GetInventoryLocationByID :one
SELECT * FROM inventory_locations WHERE id = $1;

-- name: GetInventoryLocationBySlug :one
SELECT * FROM inventory_locations WHERE slug = $1;

-- name: ListInventoryLocations :many
SELECT * FROM inventory_locations ORDER BY name;

-- name: ListInventoryLocationsByIDs :many
SELECT * FROM inventory_locations WHERE id = ANY($1::uuid[]);

-- name: UpdateInventoryLocation :one
UPDATE inventory_locations
SET name                = $2,
    is_active           = $3,
    line1               = $4,
    line2               = $5,
    city                = $6,
    region              = $7,
    postal_code         = $8,
    country_code        = $9,
    latitude            = $10,
    longitude           = $11,
    type                = $12,
    timezone            = $13,
    hours               = $14,
    email               = $15,
    phone               = $16,
    contact_name        = $17,
    external_reference  = $18,
    metadata            = $19,
    storefront_visible  = $20,
    pickup_enabled      = COALESCE(sqlc.narg(pickup_enabled)::boolean, pickup_enabled),
    pickup_instructions = COALESCE(sqlc.narg(pickup_instructions)::text, pickup_instructions),
    updated_at          = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteInventoryLocation :exec
DELETE FROM inventory_locations WHERE id = $1;

-- ==================== Channel ↔ Location junction ====================

-- name: ListChannelLocations :many
SELECT l.*, svl.priority
FROM inventory_channel_locations svl
JOIN inventory_locations l ON l.id = svl.location_id
WHERE svl.channel_id = $1
ORDER BY svl.priority ASC, l.name ASC;

-- name: DeleteChannelLocations :exec
DELETE FROM inventory_channel_locations WHERE channel_id = $1;

-- name: InsertChannelLocation :exec
INSERT INTO inventory_channel_locations (channel_id, location_id, priority)
VALUES ($1, $2, $3);

-- ==================== Inventory Items ====================

-- name: CreateInventoryItem :one
INSERT INTO inventory_items (product_id, tracked, backorderable, low_stock_threshold)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetInventoryItemByID :one
SELECT * FROM inventory_items WHERE id = $1;

-- name: GetInventoryItemByProductID :one
SELECT * FROM inventory_items WHERE product_id = $1;

-- name: GetInventoryItemByProductIDForUpdate :one
-- Row-locked read for flat-mode reservation: the sufficient-stock check on inventory_items.quantity is check-then-act, so the row stays locked for the duration of the reserving transaction
SELECT * FROM inventory_items WHERE product_id = $1 FOR UPDATE;

-- name: ListInventoryItemsByProductIDs :many
SELECT * FROM inventory_items WHERE product_id = ANY($1::uuid[]);

-- name: UpdateInventoryItem :one
UPDATE inventory_items
SET tracked              = $2,
    backorderable        = $3,
    low_stock_threshold  = $4,
    updated_at           = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteInventoryItem :exec
DELETE FROM inventory_items WHERE id = $1;

-- ==================== Inventory Levels ====================

-- name: UpsertInventoryLevel :one
INSERT INTO inventory_levels (inventory_item_id, location_id, quantity)
VALUES ($1, $2, $3)
ON CONFLICT (inventory_item_id, location_id) DO UPDATE
SET quantity   = EXCLUDED.quantity,
    updated_at = NOW()
RETURNING *;

-- name: AdjustInventoryLevelQuantity :one
-- Used when fulfillment ships goods: decrements quantity by the delta
-- Negative deltas decrement; positive deltas increment
UPDATE inventory_levels
SET quantity   = quantity + $3,
    updated_at = NOW()
WHERE inventory_item_id = $1 AND location_id = $2
RETURNING *;

-- name: AdjustInventoryLevelReserved :exec
-- Maintains the denormalized reserved counter alongside the ledger - upserts so a reservation at a not-yet-stocked location (backorder) still carries its reserved amount. $3 (delta) = -reservation.quantity_delta
INSERT INTO inventory_levels (inventory_item_id, location_id, quantity, reserved_quantity)
VALUES ($1, $2, 0, $3)
ON CONFLICT (inventory_item_id, location_id) DO UPDATE
SET reserved_quantity = inventory_levels.reserved_quantity + EXCLUDED.reserved_quantity,
    updated_at        = NOW();

-- name: RecomputeAllLevelReserved :exec
-- Reconcile: reset every level's reserved_quantity to the ledger truth (-net of its open reservations, clamped at 0) - the ledger is authoritative; this repairs counter drift manual_adjustment rows are quantity AUDIT entries (the change already lives in inventory_levels.quantity via the adjust upsert), NOT holds - counting them here wiped real holds and manufactured phantom ones
UPDATE inventory_levels l
SET reserved_quantity = GREATEST(0, COALESCE((
        SELECT -SUM(r.quantity_delta)
        FROM inventory_reservations r
        WHERE r.inventory_item_id = l.inventory_item_id
          AND r.location_id = l.location_id
          AND r.reason <> 'manual_adjustment'
    ), 0)),
    updated_at = NOW();

-- name: GetInventoryLevel :one
SELECT * FROM inventory_levels
WHERE inventory_item_id = $1 AND location_id = $2;

-- name: GetInventoryLevelForUpdate :one
-- Row-locked read used inside the reservation transaction - availability is check-then-act (level quantity + net reservations, then an insert), so concurrent reservers of the same (item, location) must serialize on the level row or they both pass the check and oversell
SELECT * FROM inventory_levels
WHERE inventory_item_id = $1 AND location_id = $2
FOR UPDATE;

-- name: ListInventoryLevelsByItem :many
SELECT * FROM inventory_levels WHERE inventory_item_id = $1;

-- name: ListInventoryLevelsByItems :many
SELECT * FROM inventory_levels WHERE inventory_item_id = ANY($1::uuid[]);

-- ==================== Reservations (append-only ledger) ====================

-- name: InsertInventoryReservation :one
INSERT INTO inventory_reservations
    (inventory_item_id, location_id, quantity_delta, reason, order_id, order_line_id, admin_id, note)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: ListReservationsForOrder :many
SELECT * FROM inventory_reservations
WHERE order_id = $1
ORDER BY created_at;

-- ==================== Low-stock dashboard ====================

-- name: ListLowStockLevels :many
-- Tracked items needing attention, unioned: (item, location) pairs at or below the item's low_stock_threshold, plus tracked items with a threshold but no levels anywhere
-- The second arm surfaces as one row with NULL location columns so the dashboard can mark it unstocked; threshold=0 means no alert wanted and is filtered from both arms
SELECT * FROM (
    SELECT
        inv_items.id                  AS inventory_item_id,
        inv_items.product_id          AS product_id,
        inv_items.low_stock_threshold AS low_stock_threshold,
        inv_items.backorderable       AS backorderable,
        NULL::uuid                    AS location_id,
        0::numeric                    AS quantity,
        inv_items.updated_at          AS level_updated_at,
        products.url                  AS product_slug,
        products.sku                  AS product_sku,
        products.quantity_decimals    AS quantity_decimals,
        products.quantity_step        AS quantity_step,
        NULL::text                    AS location_slug,
        NULL::text                    AS location_name,
        0                             AS sort_bucket
    FROM inventory_items inv_items
    JOIN catalog_products products ON products.id = inv_items.product_id
    WHERE inv_items.tracked = TRUE
      AND inv_items.low_stock_threshold > 0
      AND NOT EXISTS (
        SELECT 1 FROM inventory_levels
        WHERE inventory_item_id = inv_items.id
      )

    UNION ALL

    SELECT
        inv_items.id                  AS inventory_item_id,
        inv_items.product_id          AS product_id,
        inv_items.low_stock_threshold AS low_stock_threshold,
        inv_items.backorderable       AS backorderable,
        inv_levels.location_id        AS location_id,
        inv_levels.quantity           AS quantity,
        inv_levels.updated_at         AS level_updated_at,
        products.url                  AS product_slug,
        products.sku                  AS product_sku,
        products.quantity_decimals    AS quantity_decimals,
        products.quantity_step        AS quantity_step,
        locs.slug                     AS location_slug,
        locs.name                     AS location_name,
        1                             AS sort_bucket
    FROM inventory_items inv_items
    JOIN inventory_levels inv_levels ON inv_levels.inventory_item_id = inv_items.id
    JOIN catalog_products products ON products.id = inv_items.product_id
    JOIN inventory_locations locs ON locs.id = inv_levels.location_id
    WHERE inv_items.tracked = TRUE
      AND inv_items.low_stock_threshold > 0
      AND inv_levels.quantity <= inv_items.low_stock_threshold
) AS combined
ORDER BY sort_bucket ASC,
         (low_stock_threshold - quantity) DESC,
         product_slug ASC,
         location_slug ASC NULLS FIRST
LIMIT sqlc.arg(lim);

-- name: AdjustInventoryItemQuantity :one
-- Atomic delta on the flat-mode quantity column - positive delta adds stock (cancel/refund); negative delta subtracts (order place) The caller is responsible for checking backorderable / sufficient stock before invoking - this query unconditionally applies the delta - used by Service.Reserve and Service.ReleaseForOrder in flat mode - the CHECK (quantity >= 0) constraint on the column rejects a negative result for non-backorderable items at the DB layer
UPDATE inventory_items
SET quantity   = quantity + $2,
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: SetInventoryItemQuantity :one
-- Flat-mode write: overwrites quantity + audit fields for one product Used by syncProductInventory (ERP push) and by admin SetItemQuantity calls - upserts the row so an ERP can sync a product before the catalog seed creates its inventory_items entry
INSERT INTO inventory_items (product_id, tracked, backorderable, low_stock_threshold, quantity, last_synced_at, last_synced_source)
VALUES ($1, TRUE, FALSE, 0, $2, NOW(), $3)
ON CONFLICT (product_id) DO UPDATE
SET quantity           = EXCLUDED.quantity,
    last_synced_at     = EXCLUDED.last_synced_at,
    last_synced_source = EXCLUDED.last_synced_source,
    updated_at         = NOW()
RETURNING *;

-- name: ListLowStockItemsFlat :many
-- Flat-mode low-stock list - one row per tracked item with quantity at or below its threshold - LocationID is NULL - flat mode has no location dimension; the dashboard renders "(global)" for that column
SELECT
    inv_items.id                  AS inventory_item_id,
    inv_items.product_id          AS product_id,
    inv_items.low_stock_threshold AS low_stock_threshold,
    inv_items.backorderable       AS backorderable,
    inv_items.quantity            AS quantity,
    inv_items.updated_at          AS level_updated_at,
    products.url                  AS product_slug,
    products.sku                  AS product_sku,
    products.quantity_decimals    AS quantity_decimals,
    products.quantity_step        AS quantity_step
FROM inventory_items inv_items
JOIN catalog_products products ON products.id = inv_items.product_id
WHERE inv_items.tracked = TRUE
  AND inv_items.low_stock_threshold > 0
  AND inv_items.quantity <= inv_items.low_stock_threshold
ORDER BY (inv_items.low_stock_threshold - inv_items.quantity) DESC,
         products.url ASC
LIMIT sqlc.arg(lim);

-- name: ListPickupLocations :many
SELECT * FROM inventory_locations
WHERE pickup_enabled AND is_active
ORDER BY name;

-- name: ExportInventoryLevels :many
-- One row per (product, location) stock cell, joined to the SKU the transfer file addresses it by
SELECT p.sku,
       loc.slug AS location_slug,
       lv.quantity,
       lv.reserved_quantity,
       i.tracked,
       i.backorderable,
       i.low_stock_threshold
FROM inventory_levels lv
JOIN inventory_items i ON i.id = lv.inventory_item_id
JOIN catalog_products p ON p.id = i.product_id
JOIN inventory_locations loc ON loc.id = lv.location_id
ORDER BY p.sku, loc.slug;

-- name: ExportInventoryItemsFlat :many
-- Flat mode keeps one quantity per product with no location dimension, so it needs its own shape
SELECT p.sku,
       i.quantity,
       i.tracked,
       i.backorderable,
       i.low_stock_threshold
FROM inventory_items i
JOIN catalog_products p ON p.id = i.product_id
ORDER BY p.sku;

-- name: BackfillItemQuantityFromLevels :execrows
-- Flips multi-location -> flat: one quantity per product, taken from SALABLE (physical minus held), because flat mode's quantity IS the number customers can buy - its placement path decrements it Reserved is floored at 0 so a drifted negative can't inflate the result, matching Salable's clamp
UPDATE inventory_items i
SET quantity = COALESCE(agg.salable, 0),
    updated_at = NOW()
FROM (
    SELECT it.id AS item_id,
           SUM(GREATEST(lv.quantity - GREATEST(lv.reserved_quantity, 0), 0)) AS salable
    FROM inventory_items it
    LEFT JOIN inventory_levels lv ON lv.inventory_item_id = it.id
    GROUP BY it.id
) AS agg
WHERE i.id = agg.item_id
  AND i.quantity <> COALESCE(agg.salable, 0);

-- name: PreviewItemQuantityFromLevels :many
-- The dry run for BackfillItemQuantityFromLevels - same aggregate, no write
SELECT p.sku,
       i.quantity AS current_quantity,
       COALESCE(SUM(GREATEST(lv.quantity - GREATEST(lv.reserved_quantity, 0), 0)), 0)::numeric AS new_quantity
FROM inventory_items i
JOIN catalog_products p ON p.id = i.product_id
LEFT JOIN inventory_levels lv ON lv.inventory_item_id = i.id
GROUP BY p.sku, i.id, i.quantity
ORDER BY p.sku;

-- name: BackfillLevelsFromItemQuantity :execrows
-- Flips flat -> multi-location: parks every product's flat quantity at one target location Other locations for the same item are zeroed by ZeroLevelsOutsideLocation, or their stale quantities would add on top of what lands here
INSERT INTO inventory_levels (inventory_item_id, location_id, quantity)
SELECT i.id, $1, i.quantity
FROM inventory_items i
ON CONFLICT (inventory_item_id, location_id)
DO UPDATE SET quantity = EXCLUDED.quantity, updated_at = NOW();

-- name: ZeroLevelsOutsideLocation :execrows
-- Clears stock parked at every OTHER location, so a flat -> multi flip lands the whole quantity in
-- one place instead of summing with whatever the pre-flat era left behind
UPDATE inventory_levels
SET quantity = 0, updated_at = NOW()
WHERE location_id <> $1
  AND quantity <> 0;

-- name: PruneSettledReservations :one
-- Deletes every order's chain that nets to zero and has been quiet since the cutoff, in one statement, so the settled check and the delete read the same snapshot
WITH settled AS (
    SELECT order_id
    FROM inventory_reservations
    WHERE order_id IS NOT NULL
    GROUP BY order_id
    HAVING SUM(quantity_delta) = 0 AND MAX(created_at) < sqlc.arg(cutoff)::timestamptz
), gone AS (
    DELETE FROM inventory_reservations r
    USING settled
    WHERE r.order_id = settled.order_id
    RETURNING r.order_id
)
SELECT count(DISTINCT order_id)::bigint FROM gone;

-- name: LockOrderLedger :exec
-- Serialises every read-then-compensate pass over one order's ledger - a replayed or racing event must see the winner's rows, which READ COMMITTED alone does not promise
SELECT pg_advisory_xact_lock(hashtextextended('inventory-order:' || sqlc.arg(order_id)::uuid::text, 0));

-- name: MarkFlatRestored :execrows
INSERT INTO inventory_flat_restores (order_id, reason) VALUES ($1, $2)
ON CONFLICT (order_id) DO NOTHING;

-- name: EnsureInventoryLevel :exec
INSERT INTO inventory_levels (inventory_item_id, location_id, quantity) VALUES ($1, $2, 0)
ON CONFLICT (inventory_item_id, location_id) DO NOTHING;

-- name: CountLocationCommitments :one
-- What deleting a location would silently destroy: stock on its shelves, and order holds whose chains are still open there
SELECT (
    (SELECT count(*) FROM inventory_levels l
     WHERE l.location_id = sqlc.arg(location_id)::uuid AND (l.quantity <> 0 OR l.reserved_quantity <> 0)) +
    (SELECT count(*) FROM (
        SELECT 1 FROM inventory_reservations r
        WHERE r.location_id = sqlc.arg(location_id)::uuid AND r.order_id IS NOT NULL AND r.reason <> 'manual_adjustment'
        GROUP BY r.order_id, r.inventory_item_id, r.order_line_id
        HAVING SUM(r.quantity_delta) <> 0
    ) open_chains)
)::bigint AS total;

-- name: ListPublicInventoryLocations :many
SELECT l.* FROM inventory_locations l
JOIN inventory_channel_locations cl ON cl.location_id = l.id
WHERE cl.channel_id = $1 AND l.is_active AND l.storefront_visible
ORDER BY lower(l.name), l.id;

-- name: ListFilteredInventoryLocations :many
SELECT * FROM inventory_locations
WHERE (sqlc.arg(search)::text = ''
 OR name ILIKE '%' || escape_like(sqlc.arg(search)::text) || '%'
 OR slug ILIKE '%' || escape_like(sqlc.arg(search)::text) || '%'
 OR city ILIKE '%' || escape_like(sqlc.arg(search)::text) || '%')
 AND (cardinality(sqlc.arg(types)::text[]) = 0 OR type = ANY(sqlc.arg(types)::text[]))
 AND (sqlc.arg(city)::text = '' OR city ILIKE '%' || escape_like(sqlc.arg(city)::text) || '%')
 AND (sqlc.narg(active)::boolean IS NULL OR is_active = sqlc.narg(active))
 AND (sqlc.narg(visible)::boolean IS NULL OR storefront_visible = sqlc.narg(visible))
 AND (sqlc.narg(pickup)::boolean IS NULL OR pickup_enabled = sqlc.narg(pickup))
ORDER BY lower(name), id
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: CountFilteredInventoryLocations :one
SELECT COUNT(*) FROM inventory_locations
WHERE (sqlc.arg(search)::text = ''
 OR name ILIKE '%' || escape_like(sqlc.arg(search)::text) || '%'
 OR slug ILIKE '%' || escape_like(sqlc.arg(search)::text) || '%'
 OR city ILIKE '%' || escape_like(sqlc.arg(search)::text) || '%')
 AND (cardinality(sqlc.arg(types)::text[]) = 0 OR type = ANY(sqlc.arg(types)::text[]))
 AND (sqlc.arg(city)::text = '' OR city ILIKE '%' || escape_like(sqlc.arg(city)::text) || '%')
 AND (sqlc.narg(active)::boolean IS NULL OR is_active = sqlc.narg(active))
 AND (sqlc.narg(visible)::boolean IS NULL OR storefront_visible = sqlc.narg(visible))
 AND (sqlc.narg(pickup)::boolean IS NULL OR pickup_enabled = sqlc.narg(pickup));
