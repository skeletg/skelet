-- Postgres backend for verification / password-reset tokens,
-- only used when Valkey isn't configured

-- name: PutAuthToken :exec
-- Upsert so re-issuing a token key refreshes subject + expiry instead of erroring
-- (a caller re-Create is legal, even if the key collision is astronomically rare)
INSERT INTO auth_tokens (token_key, subject_id, expires_at)
VALUES ($1, $2, $3)
ON CONFLICT (token_key) DO UPDATE
SET subject_id = EXCLUDED.subject_id,
    expires_at = EXCLUDED.expires_at;

-- name: GetAuthToken :one
-- Expiry is filtered here, not left to the prune sweep, so a token can't validate
-- past its TTL even if pruning is behind
SELECT subject_id FROM auth_tokens
WHERE token_key = $1 AND expires_at > now();

-- name: DeleteAuthToken :exec
DELETE FROM auth_tokens WHERE token_key = $1;

-- name: PruneExpiredAuthTokens :execrows
DELETE FROM auth_tokens WHERE expires_at <= now();
