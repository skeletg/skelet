-- name: ListAdminPreferences :many
SELECT key, value, updated_at
FROM admin_preferences
WHERE admin_id = sqlc.arg(admin_id)
ORDER BY key;

-- name: GetAdminPreference :one
-- One key, for server-side consumers - the inbox reads a muted-categories list on
-- every query, and pulling an admin's whole preference set for it would be wasteful
SELECT key, value, updated_at
FROM admin_preferences
WHERE admin_id = sqlc.arg(admin_id) AND key = sqlc.arg(key);

-- name: UpsertAdminPreference :exec
INSERT INTO admin_preferences (admin_id, key, value)
VALUES (sqlc.arg(admin_id), sqlc.arg(key), sqlc.arg(value))
ON CONFLICT (admin_id, key)
DO UPDATE SET value = EXCLUDED.value, updated_at = now();

-- name: DeleteAdminPreference :exec
DELETE FROM admin_preferences
WHERE admin_id = sqlc.arg(admin_id) AND key = sqlc.arg(key);

-- name: CountOtherAdminPreferences :one
-- Keys the admin holds besides this one, so the cap blocks a new key without blocking an update to an existing one
SELECT count(*) FROM admin_preferences
WHERE admin_id = sqlc.arg(admin_id) AND key <> sqlc.arg(key);
