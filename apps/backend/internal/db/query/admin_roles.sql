-- name: CreateAdminRole :one
INSERT INTO admin_roles (name, slug, description) VALUES ($1, $2, $3) RETURNING *;

-- name: UpdateAdminRole :one
UPDATE admin_roles SET name = $2, description = $3 WHERE id = $1 RETURNING *;

-- name: DeleteAdminRole :exec
DELETE FROM admin_roles WHERE id = $1;

-- name: GetAdminRoleByID :one
SELECT * FROM admin_roles WHERE id = $1;

-- name: GetAdminRoleBySlug :one
SELECT * FROM admin_roles WHERE slug = $1;

-- name: ListAdminRolesWithMemberCount :many
SELECT r.id, r.name, r.slug, r.description, r.created_at,
       COALESCE(c.member_count, 0)::bigint AS member_count
FROM admin_roles r
LEFT JOIN (
    SELECT role_id, COUNT(*) AS member_count
    FROM admin_user_roles
    GROUP BY role_id
) c ON c.role_id = r.id
ORDER BY r.name;

-- name: CountRoleMembers :one
SELECT COUNT(*) FROM admin_user_roles WHERE role_id = $1;

-- name: GetAdminUserRoles :many
SELECT r.* FROM admin_roles r
JOIN admin_user_roles aur ON aur.role_id = r.id
WHERE aur.admin_user_id = $1
ORDER BY r.name;

-- name: DeleteAdminUserRoles :exec
DELETE FROM admin_user_roles WHERE admin_user_id = $1;

-- name: AddAdminUserRole :exec
INSERT INTO admin_user_roles (admin_user_id, role_id) VALUES ($1, $2);

-- ============================================================
-- Permission scopes - single source of truth for what a role grants
-- Each row is one (permission, channel?, locale?) tuple - NULL in
-- either scope column means "any"
-- ============================================================

-- name: GetRolePermissionScopes :many
-- Returns one row per scope, with the joined permission hydrated for
-- the service to render - ORDER BY keeps the display stable
SELECT
    arps.role_id,
    arps.permission_id,
    arps.channel_id,
    arps.locale,
    p.slug       AS permission_slug,
    p.name       AS permission_name,
    p.group_id   AS permission_group_id,
    p.sort_order AS permission_sort_order
FROM admin_role_permission_scopes arps
JOIN permissions p ON p.id = arps.permission_id
WHERE arps.role_id = $1
ORDER BY p.sort_order, p.name,
         arps.channel_id NULLS FIRST,
         arps.locale     NULLS FIRST;

-- name: GetRolePermissionScopesByRoleIDs :many
-- Batch of GetRolePermissionScopes across many roles - list hydration groups
-- the result by role_id (each row carries it) instead of firing one query per
-- role, killing the roles x scopes N+1 on the admin user / roles lists
SELECT
    arps.role_id,
    arps.permission_id,
    arps.channel_id,
    arps.locale,
    p.slug       AS permission_slug,
    p.name       AS permission_name,
    p.group_id   AS permission_group_id,
    p.sort_order AS permission_sort_order
FROM admin_role_permission_scopes arps
JOIN permissions p ON p.id = arps.permission_id
WHERE arps.role_id = ANY(sqlc.arg(role_ids)::uuid[])
ORDER BY p.sort_order, p.name,
         arps.channel_id NULLS FIRST,
         arps.locale     NULLS FIRST;

-- name: DeleteRolePermissionScopes :exec
DELETE FROM admin_role_permission_scopes WHERE role_id = $1;

-- name: InsertRolePermissionScope :exec
INSERT INTO admin_role_permission_scopes (role_id, permission_id, channel_id, locale)
VALUES ($1, $2, $3, $4)
ON CONFLICT DO NOTHING;

-- name: ListAdminUserGrants :many
-- Every permission grant for an admin across all their roles: one row
-- per (slug, channel, locale) scope - powers the per-request grant
-- snapshot (internal/admin/grants.go) so a request answers Has* /
-- ListPermitted* in memory from one query instead of one query per
-- check - collapsing the per-product N+1 in the product list
SELECT p.slug, arps.channel_id, arps.locale
FROM admin_user_roles aur
JOIN admin_role_permission_scopes arps ON arps.role_id = aur.role_id
JOIN permissions p ON p.id = arps.permission_id
WHERE aur.admin_user_id = $1;
