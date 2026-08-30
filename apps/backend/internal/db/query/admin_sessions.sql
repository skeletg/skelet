-- name: CreateAdminSession :one
INSERT INTO admin_sessions (admin_id, token_hash, ip_address, user_agent)
VALUES ($1, $2, $3, $4)
ON CONFLICT (token_hash) DO UPDATE
    SET last_active_at = NOW()
RETURNING id;

-- name: DeleteAdminSession :exec
DELETE FROM admin_sessions WHERE token_hash = $1;

-- name: DeleteAdminSessionsByAdminID :exec
DELETE FROM admin_sessions WHERE admin_id = $1;

-- name: DeleteAdminSessionsExcept :exec
DELETE FROM admin_sessions WHERE admin_id = $1 AND token_hash <> $2;

-- name: ListAdminSessionsByAdminID :many
SELECT * FROM admin_sessions WHERE admin_id = $1 ORDER BY last_active_at DESC;
