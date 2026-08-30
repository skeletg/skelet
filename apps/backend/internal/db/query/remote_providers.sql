-- name: CreateRemoteProvider :one
INSERT INTO remote_providers (
    kind, slug, name, target_url, signing_secret, config,
    timeout_ms, cache_ttl_s, failure_policy, channel_id
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING *;

-- name: GetRemoteProviderByID :one
SELECT * FROM remote_providers WHERE id = $1;

-- name: GetRemoteProviderBySlug :one
SELECT * FROM remote_providers WHERE kind = $1 AND slug = $2;

-- name: ListRemoteProviders :many
SELECT * FROM remote_providers ORDER BY kind, slug;

-- name: ListActiveRemoteProvidersByKind :many
-- Global rows (channel_id IS NULL) apply to every store; scoped rows only
-- to theirs - passing NULL channel_id returns global rows only
SELECT * FROM remote_providers
WHERE is_active = TRUE
  AND kind = sqlc.arg(kind)::text
  AND (channel_id IS NULL OR channel_id = sqlc.narg(channel_id)::uuid)
ORDER BY slug;

-- name: UpdateRemoteProvider :one
UPDATE remote_providers
SET name = $2, target_url = $3, config = $4, timeout_ms = $5,
    cache_ttl_s = $6, failure_policy = $7, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: SetRemoteProviderActive :one
UPDATE remote_providers SET is_active = $2, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: UpdateRemoteProviderSecret :one
UPDATE remote_providers SET signing_secret = $2, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: GetRemoteProviderSecret :one
SELECT signing_secret FROM remote_providers WHERE id = $1;

-- name: DeleteRemoteProvider :exec
DELETE FROM remote_providers WHERE id = $1;
