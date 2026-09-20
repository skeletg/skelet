-- name: CreateIntegration :one
INSERT INTO integrations (name, description, token_hash, expires_at, channel_id)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetIntegrationByID :one
SELECT * FROM integrations WHERE id = $1;

-- name: GetIntegrationByHash :one
SELECT * FROM integrations WHERE token_hash = $1;

-- name: ListIntegrations :many
SELECT * FROM integrations ORDER BY created_at DESC;

-- name: UpdateIntegration :one
UPDATE integrations
SET name = $2, description = $3, expires_at = $4, channel_id = $5
WHERE id = $1
RETURNING *;

-- name: SetIntegrationActive :one
UPDATE integrations SET is_active = $2 WHERE id = $1 RETURNING *;

-- name: UpdateIntegrationTokenHash :one
UPDATE integrations SET token_hash = $2 WHERE id = $1 RETURNING *;

-- name: DeleteIntegration :exec
DELETE FROM integrations WHERE id = $1;

-- name: GetIntegrationPermissions :many
SELECT p.* FROM permissions p
JOIN integration_permissions ip ON ip.permission_id = p.id
WHERE ip.integration_id = $1
ORDER BY p.sort_order, p.name;

-- name: AddIntegrationPermission :exec
INSERT INTO integration_permissions (integration_id, permission_id) VALUES ($1, $2);

-- name: DeleteIntegrationPermissions :exec
DELETE FROM integration_permissions WHERE integration_id = $1;

-- name: HasIntegrationPermission :one
SELECT EXISTS (
    SELECT 1 FROM integration_permissions ip
    JOIN permissions p ON p.id = ip.permission_id
    WHERE ip.integration_id = $1 AND p.slug = $2
) AS has_permission;
