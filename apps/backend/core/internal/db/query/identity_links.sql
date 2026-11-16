-- name: GetIdentityLink :one
SELECT * FROM identity_links
WHERE  issuer = sqlc.arg(issuer) AND subject = sqlc.arg(subject) AND kind = sqlc.arg(kind)
  AND  channel_id IS NOT DISTINCT FROM sqlc.narg(channel_id);

-- name: CreateIdentityLink :one
INSERT INTO identity_links (issuer, subject, kind, channel_id, user_id, email)
VALUES (sqlc.arg(issuer), sqlc.arg(subject), sqlc.arg(kind), sqlc.narg(channel_id), sqlc.arg(user_id), sqlc.arg(email))
ON CONFLICT (issuer, subject, kind, coalesce(channel_id, '00000000-0000-0000-0000-000000000000')) DO NOTHING
RETURNING *;

-- name: TouchIdentityLink :exec
UPDATE identity_links SET last_used_at = now(), email = sqlc.arg(email) WHERE id = sqlc.arg(id);

-- name: DeleteIdentityLink :exec
DELETE FROM identity_links WHERE id = sqlc.arg(id);

-- name: DeleteIdentityLinksForUser :exec
DELETE FROM identity_links WHERE kind = sqlc.arg(kind) AND user_id = sqlc.arg(user_id);

-- name: ListIdentityLinksForUser :many
SELECT * FROM identity_links WHERE kind = sqlc.arg(kind) AND user_id = sqlc.arg(user_id) ORDER BY created_at;

-- name: DeleteIdentityLinkForUser :execrows
DELETE FROM identity_links WHERE id = sqlc.arg(id) AND kind = sqlc.arg(kind) AND user_id = sqlc.arg(user_id);

-- name: DeleteIdentityLinksForUserIssuer :exec
DELETE FROM identity_links WHERE kind = sqlc.arg(kind) AND user_id = sqlc.arg(user_id) AND issuer = sqlc.arg(issuer);
