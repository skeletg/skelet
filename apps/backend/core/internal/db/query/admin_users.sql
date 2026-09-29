-- name: CreateAdminUser :one
INSERT INTO admin_users (email, password_hash, first_name, last_name, is_superuser)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetAdminUserByEmail :one
SELECT * FROM admin_users WHERE lower(email) = lower($1) LIMIT 1;

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

-- name: EnableAdminTotp :execrows
-- Promotes the pending secret + stores hashed recovery codes
UPDATE admin_users
SET totp_secret = totp_pending_secret,
    totp_pending_secret = NULL,
    totp_enabled = TRUE,
    totp_recovery_codes = $2,
    totp_last_step = $3,
    totp_failed_attempts = 0,
    totp_locked_until = NULL,
    updated_at = NOW()
WHERE id = $1 AND totp_pending_secret IS NOT NULL AND NOT totp_enabled;

-- name: DisableAdminTotp :exec
UPDATE admin_users
SET totp_secret = NULL,
    totp_pending_secret = NULL,
    totp_enabled = FALSE,
    totp_recovery_codes = NULL,
    totp_last_step = NULL,
    totp_failed_attempts = 0,
    totp_locked_until = NULL,
    updated_at = NOW()
WHERE id = $1;

-- name: RedeemAdminTotpRecoveryCode :execrows
-- Compare-and-swap on the code list, so two logins redeeming off the same read can't both succeed or write a spent code back
UPDATE admin_users SET totp_recovery_codes = sqlc.arg(remaining), updated_at = NOW()
WHERE id = sqlc.arg(id) AND totp_recovery_codes = sqlc.arg(expected);

-- name: AcceptAdminTotpStep :execrows
-- Zero rows means the step was already used, which is a replay
UPDATE admin_users SET totp_last_step = sqlc.arg(step), totp_failed_attempts = 0, totp_locked_until = NULL
WHERE id = sqlc.arg(id) AND (totp_last_step IS NULL OR totp_last_step < sqlc.arg(step));

-- name: ClearAdminTotpFailures :exec
UPDATE admin_users SET totp_failed_attempts = 0, totp_locked_until = NULL WHERE id = $1;

-- name: ClaimAdminTotpAttempt :execrows
-- Taken before a code is checked, so parallel guesses serialise on the row instead of all passing one stale read - zero rows means locked out, and the claim that reaches the limit starts the lockout; a lapsed lockout starts a fresh count
UPDATE admin_users
SET totp_failed_attempts = CASE WHEN totp_locked_until IS NOT NULL THEN 1 ELSE totp_failed_attempts + 1 END,
    totp_locked_until = CASE
        WHEN (CASE WHEN totp_locked_until IS NOT NULL THEN 1 ELSE totp_failed_attempts + 1 END) >= sqlc.arg(max_attempts)::int
        THEN NOW() + make_interval(secs => sqlc.arg(lock_seconds)::int)
    END
WHERE id = sqlc.arg(id) AND (totp_locked_until IS NULL OR totp_locked_until <= NOW());

-- name: RecordAdminLogin :one
-- Stamps this login and returns BOTH the previous value (so the caller can tell a first login from a later one) and the new one (so the login payload can carry an accurate "last seen" without a re-fetch) - a CTE rather than two statements, since a read-then-write would let two concurrent logins both believe they were first
WITH prev AS (
    SELECT last_login_at FROM admin_users WHERE admin_users.id = sqlc.arg(id)
)
UPDATE admin_users
SET last_login_at = NOW()
WHERE admin_users.id = sqlc.arg(id)
RETURNING (SELECT prev.last_login_at FROM prev) AS previous_login_at, admin_users.last_login_at;
