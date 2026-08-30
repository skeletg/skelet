-- name: SetProductMediaRole :exec
INSERT INTO catalog_product_media_roles (product_id, role, media_id)
VALUES ($1, $2, $3)
ON CONFLICT (product_id, role) DO UPDATE SET media_id = EXCLUDED.media_id;

-- name: ClearProductMediaRole :exec
DELETE FROM catalog_product_media_roles
WHERE product_id = $1 AND role = $2;

-- name: GetProductMediaRoles :many
SELECT role, media_id FROM catalog_product_media_roles
WHERE product_id = $1;

-- name: GetProductMediaRolesByProductIDs :many
SELECT product_id, role, media_id
FROM catalog_product_media_roles
WHERE product_id = ANY(sqlc.arg(product_ids)::uuid[]);
