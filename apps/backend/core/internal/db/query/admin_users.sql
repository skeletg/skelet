-- name: CreateAdminUser :one
INSERT INTO admin_users (email, password_hash, first_name, last_name, is_superuser)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetAdminUserByEmail :one
SELECT * FROM admin_users WHERE email = $1 LIMIT 1;

-- name: GetAdminUserByID :one
SELECT * FROM admin_users WHERE id = $1 LIMIT 1;

-- name: ListAdminUsers :many
SELECT * FROM admin_users ORDER BY created_at DESC;

-- name: UpdateAdminUser :one
UPDATE admin_users
SET first_name = $2, last_name = $3, is_active = $4, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: UpdateAdminUserPassword :exec
UPDATE admin_users SET password_hash = $2, updated_at = NOW() WHERE id = $1;

-- name: DeleteAdminUser :exec
DELETE FROM admin_users WHERE id = $1;

-- name: SetAdminTotpPending :exec
UPDATE admin_users SET totp_pending_secret = $2, updated_at = NOW() WHERE id = $1;

-- name: EnableAdminTotp :exec
-- Promotes the pending secret + stores hashed recovery codes
UPDATE admin_users
SET totp_secret = totp_pending_secret,
    totp_pending_secret = NULL,
    totp_enabled = TRUE,
    totp_recovery_codes = $2,
    updated_at = NOW()
WHERE id = $1 AND totp_pending_secret IS NOT NULL;

-- name: DisableAdminTotp :exec
UPDATE admin_users
SET totp_secret = NULL,
    totp_pending_secret = NULL,
    totp_enabled = FALSE,
    totp_recovery_codes = NULL,
    updated_at = NOW()
WHERE id = $1;

-- name: SetAdminTotpRecoveryCodes :exec
UPDATE admin_users SET totp_recovery_codes = $2, updated_at = NOW() WHERE id = $1;

-- name: RecordAdminLogin :one
-- Stamps this login and returns BOTH the previous value (so the caller can tell a first login from a later one) and the new one (so the login payload can carry an accurate "last seen" without a re-fetch) - a CTE rather than two statements, since a read-then-write would let two concurrent logins both believe they were first
WITH prev AS (
    SELECT last_login_at FROM admin_users WHERE admin_users.id = sqlc.arg(id)
)
UPDATE admin_users
SET last_login_at = NOW()
WHERE admin_users.id = sqlc.arg(id)
RETURNING (SELECT prev.last_login_at FROM prev) AS previous_login_at, admin_users.last_login_at;
