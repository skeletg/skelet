-- name: CreateAdminSession :one
INSERT INTO admin_sessions (admin_id, token_hash, ip_address, user_agent)
VALUES ($1, $2, $3, $4)
ON CONFLICT (token_hash) DO UPDATE
    SET last_active_at = NOW()
RETURNING id;

-- The three delete paths return the ids they removed, so a killed session can be published to services that
-- authorize from a token instead of this table and cannot see the row disappear

-- name: DeleteAdminSession :many
DELETE FROM admin_sessions WHERE token_hash = $1 RETURNING id;

-- name: DeleteAdminSessionsByAdminID :many
DELETE FROM admin_sessions WHERE admin_id = $1 RETURNING id;

-- name: DeleteAdminSessionsExcept :many
DELETE FROM admin_sessions WHERE admin_id = $1 AND token_hash <> $2 RETURNING id;

-- name: ListAdminSessionsByAdminID :many
SELECT * FROM admin_sessions WHERE admin_id = $1 ORDER BY last_active_at DESC;
