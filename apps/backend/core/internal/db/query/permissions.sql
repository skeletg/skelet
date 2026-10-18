-- name: ListPermissionGroups :many
SELECT * FROM permission_groups ORDER BY sort_order, name;

-- name: ListPermissionsByGroupID :many
SELECT * FROM permissions WHERE group_id = $1 ORDER BY sort_order, name;

-- name: GetPermissionBySlug :one
SELECT * FROM permissions WHERE slug = $1;

-- name: UpsertPermissionGroup :one
-- Used by permission.Sync at server startup to keep permission_groups in sync with the code-defined registry - inserts when missing; updates name/sort_order when the registered copy has changed
INSERT INTO permission_groups (slug, name, sort_order) VALUES ($1, $2, $3)
ON CONFLICT (slug) DO UPDATE
  SET name = EXCLUDED.name, sort_order = EXCLUDED.sort_order
RETURNING *;

-- name: UpsertPermission :one
-- Same idea for permissions. group_id is resolved from the permission_groups
-- row created/upserted just before this call in the same tx
INSERT INTO permissions (group_id, slug, name, sort_order) VALUES ($1, $2, $3, $4)
ON CONFLICT (slug) DO UPDATE
  SET group_id = EXCLUDED.group_id, name = EXCLUDED.name, sort_order = EXCLUDED.sort_order
RETURNING *;

-- name: GrantAllPermissionsToAdministrator :exec
-- Ensures the administrator role holds every permission at global scope (channel + locale both NULL) - run at startup after upserting registered permissions so newly-registered ones are immediately available to administrators - idempotent via the unique index
INSERT INTO admin_role_permission_scopes (role_id, permission_id, channel_id, locale)
SELECT r.id, p.id, NULL, NULL
FROM admin_roles r CROSS JOIN permissions p
WHERE r.slug = 'administrator'
ON CONFLICT DO NOTHING;

-- name: GetAdminUserPermissions :many
-- Effective slugs an admin holds (DISTINCT across roles + scopes) Scope info isn't returned here - callers that need it use the scope-aware checks (HasAdminUserChannelPermission etc.)
SELECT DISTINCT p.id, p.group_id, p.name, p.slug, p.sort_order
FROM permissions p
JOIN admin_role_permission_scopes arps ON arps.permission_id = p.id
JOIN admin_user_roles aur ON aur.role_id = arps.role_id
WHERE aur.admin_user_id = $1
ORDER BY p.sort_order, p.name;
