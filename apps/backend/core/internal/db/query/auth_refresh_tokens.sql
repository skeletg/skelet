-- Postgres backend for refresh tokens, only used when Valkey
-- isn't configured

-- name: PutRefreshToken :exec
INSERT INTO auth_refresh_tokens (token_hash, scope, owner_id, expires_at)
VALUES ($1, $2, $3, $4)
ON CONFLICT (token_hash) DO UPDATE
SET scope = EXCLUDED.scope,
    owner_id = EXCLUDED.owner_id,
    expires_at = EXCLUDED.expires_at,
    successor_token = NULL,
    grace_expires_at = NULL,
    rotated_at = NULL;

-- name: GetLiveRefreshToken :one
-- Live = not expired and not yet rotated - rotated-but-unexpired rows stay for the
-- tripwire, so `rotated_at IS NULL` is what makes this still usable
SELECT owner_id FROM auth_refresh_tokens
WHERE token_hash = $1 AND scope = $2
  AND expires_at > now() AND rotated_at IS NULL;

-- name: ClaimRefreshTokenForRotate :one
-- The rotation gate - one atomic UPDATE, and only ONE caller can match `rotated_at IS NULL` - winner gets a row, losers match nothing and fall to the grace/replay read below - same guarantee as the Valkey Lua script, no lock Consuming = stamping rotated_at (arms the tripwire) and parking the raw successor for the grace window
UPDATE auth_refresh_tokens
SET rotated_at = now(),
    successor_token = $3,
    grace_expires_at = $4
WHERE token_hash = $1 AND scope = $2
  AND expires_at > now() AND rotated_at IS NULL
RETURNING owner_id;

-- name: GetRotatedRefreshToken :one
-- For a token that lost the claim - tells "someone just rotated this, here's the successor" (grace) from "this was consumed a while ago and is being replayed" (tripwire)
SELECT owner_id, successor_token, grace_expires_at, rotated_at
FROM auth_refresh_tokens
WHERE token_hash = $1 AND scope = $2;

-- name: ConsumeRefreshToken :one
-- Single-use for the non-rotating paths (ValidateAndRevoke) - delete and return
-- the owner in one statement, so two concurrent callers can't both succeed
DELETE FROM auth_refresh_tokens
WHERE token_hash = $1 AND scope = $2
  AND expires_at > now() AND rotated_at IS NULL
RETURNING owner_id;

-- name: DeleteRefreshToken :exec
DELETE FROM auth_refresh_tokens WHERE token_hash = $1 AND scope = $2;

-- name: DeleteRefreshTokensByOwner :exec
DELETE FROM auth_refresh_tokens WHERE scope = $1 AND owner_id = $2;

-- name: DeleteRefreshTokensByOwnerExcept :exec
DELETE FROM auth_refresh_tokens
WHERE scope = $1 AND owner_id = $2 AND token_hash <> $3;

-- name: PruneExpiredRefreshTokens :execrows
-- Keep a rotated row until its reuse window lapses, so replaying a token that expired after rotation still trips the tripwire. $1 is the reuse-window cutoff The cast keeps the param a plain time.Time not NullTime - it's always supplied
DELETE FROM auth_refresh_tokens
WHERE expires_at <= now()
  AND (rotated_at IS NULL OR rotated_at <= sqlc.arg(reuse_cutoff)::timestamptz);
