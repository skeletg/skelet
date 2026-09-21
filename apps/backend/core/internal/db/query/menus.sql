-- ==================== Menus ====================

-- name: CreateMenu :one
INSERT INTO menus (slug, name)
VALUES ($1, $2)
RETURNING *;

-- name: GetMenuByID :one
SELECT * FROM menus WHERE id = $1;

-- name: GetMenuBySlug :one
SELECT * FROM menus WHERE slug = $1;

-- name: ListMenus :many
SELECT m.*, COALESCE(c.count, 0)::bigint AS item_count
FROM menus m
LEFT JOIN (
    SELECT menu_id, COUNT(*) AS count FROM menu_items GROUP BY menu_id
) c ON c.menu_id = m.id
ORDER BY m.name;

-- name: UpdateMenu :one
UPDATE menus
SET slug = $2, name = $3, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteMenu :exec
DELETE FROM menus WHERE id = $1;

-- ==================== Menu items ====================

-- name: CreateMenuItem :one
INSERT INTO menu_items (
    menu_id, parent_id, label, target_type, target_id, url, sort_order
) VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetMenuItemByID :one
SELECT * FROM menu_items WHERE id = $1;

-- name: ListMenuItemsByMenu :many
-- Flat list of every item in a menu, ordered by parent_id then sort_order - service-side code rebuilds the tree in Go because recursive CTEs would force a re-sort pass per layer anyway
SELECT * FROM menu_items
WHERE menu_id = $1
ORDER BY COALESCE(parent_id::text, ''), sort_order, created_at;

-- name: UpdateMenuItem :one
UPDATE menu_items
SET label       = $2,
    target_type = $3,
    target_id   = $4,
    url         = $5,
    parent_id   = $6,
    sort_order  = $7,
    updated_at  = NOW()
WHERE id = $1
RETURNING *;

-- name: UpdateMenuItemSortOrder :exec
-- Cheap, single-column update used by reorder mutations so the
-- caller doesn't have to round-trip the full payload per item
UPDATE menu_items
SET sort_order = $2, updated_at = NOW()
WHERE id = $1;

-- name: DeleteMenuItem :exec
DELETE FROM menu_items WHERE id = $1;

-- ==================== Menu → store assignments (many-to-many) ====================

-- name: ListMenuChannelAssignments :many
SELECT channel_id FROM menu_channel_assignments
WHERE menu_id = $1
ORDER BY channel_id;

-- name: ListMenuChannelAssignmentsByMenuIDs :many
-- Batched loader used by the admin list query - one row per (menu_id, channel_id) pair so the resolver folds them into per-menu arrays without N+1 fetches
SELECT menu_id, channel_id FROM menu_channel_assignments
WHERE menu_id = ANY(sqlc.arg(menu_ids)::uuid[])
ORDER BY menu_id, channel_id;

-- name: InsertMenuChannelAssignment :exec
INSERT INTO menu_channel_assignments (menu_id, channel_id)
VALUES ($1, $2)
ON CONFLICT (menu_id, channel_id) DO NOTHING;

-- name: DeleteMenuChannelAssignment :exec
DELETE FROM menu_channel_assignments
WHERE menu_id = $1 AND channel_id = $2;

-- name: DeleteAllMenuChannelAssignments :exec
DELETE FROM menu_channel_assignments
WHERE menu_id = $1;

-- name: GetMenuBySlugForChannel :one
-- Storefront-side fetch - returns the menu only when the request's store is in the menu's assignment set - without an assignment row the menu is treated as a draft and is invisible
SELECT m.*
FROM menus m
JOIN menu_channel_assignments a ON a.menu_id = m.id
WHERE m.slug = $1 AND a.channel_id = $2
LIMIT 1;

-- ==================== Menu item translations ====================

-- name: ListMenuItemTranslations :many
SELECT * FROM menu_item_translations
WHERE item_id = $1
ORDER BY locale;

-- name: ListMenuItemTranslationsByItemIDs :many
-- Batched loader for both the admin detail view (full translation matrix) and the front renderer (pick the request locale's label per item)
SELECT * FROM menu_item_translations
WHERE item_id = ANY(sqlc.arg(item_ids)::uuid[])
ORDER BY item_id, locale;

-- name: UpsertMenuItemTranslation :one
INSERT INTO menu_item_translations (item_id, locale, label)
VALUES ($1, $2, $3)
ON CONFLICT (item_id, locale) DO UPDATE
SET label      = EXCLUDED.label,
    updated_at = NOW()
RETURNING *;

-- name: DeleteMenuItemTranslation :exec
DELETE FROM menu_item_translations
WHERE item_id = $1 AND locale = $2;
