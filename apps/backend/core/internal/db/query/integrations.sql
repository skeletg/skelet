-- name: CreateIntegration :one
INSERT INTO integrations (name, description, token_hash, expires_at, channel_id)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetIntegrationByID :one
SELECT * FROM integrations WHERE id = $1;

-- name: GetIntegrationByIDForUpdate :one
SELECT * FROM integrations WHERE id = $1 FOR UPDATE;

-- One round trip per machine API request: the row and its grant set together
-- name: GetIntegrationByHash :one
SELECT sqlc.embed(i),
       COALESCE(array_agg(p.id ORDER BY p.sort_order, p.name) FILTER (WHERE p.id IS NOT NULL), '{}')::uuid[] AS permission_ids,
       COALESCE(array_agg(p.slug ORDER BY p.sort_order, p.name) FILTER (WHERE p.id IS NOT NULL), '{}')::text[] AS permission_slugs,
       COALESCE(array_agg(p.name ORDER BY p.sort_order, p.name) FILTER (WHERE p.id IS NOT NULL), '{}')::text[] AS permission_names
FROM integrations i
LEFT JOIN integration_permissions ip ON ip.integration_id = i.id
LEFT JOIN permissions p ON p.id = ip.permission_id
WHERE i.token_hash = $1
GROUP BY i.id;

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

-- name: DeleteIntegration :execrows
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

-- name: ListAllIntegrationPermissions :many
SELECT ip.integration_id, p.id, p.slug, p.name FROM permissions p
JOIN integration_permissions ip ON ip.permission_id = p.id
ORDER BY ip.integration_id, p.sort_order, p.name;
