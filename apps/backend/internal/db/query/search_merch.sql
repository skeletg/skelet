-- name: ListSearchSynonyms :many
SELECT * FROM search_synonyms
WHERE  channel_id IS NULL OR channel_id = $1
ORDER  BY created_at;

-- name: InsertSearchSynonym :one
INSERT INTO search_synonyms (channel_id, terms)
VALUES ($1, $2)
RETURNING *;

-- name: DeleteSearchSynonym :exec
DELETE FROM search_synonyms WHERE id = $1 AND channel_id = $2;

-- name: ListSearchPins :many
SELECT * FROM search_pins
WHERE  channel_id = $1 AND (sqlc.arg(q)::text = '' OR query = sqlc.arg(q)::text)
ORDER  BY query, position;

-- name: UpsertSearchPin :one
INSERT INTO search_pins (channel_id, query, product_id, position)
VALUES ($1, $2, $3, $4)
ON CONFLICT (channel_id, query, product_id) DO UPDATE SET position = EXCLUDED.position
RETURNING *;

-- name: DeleteSearchPin :exec
DELETE FROM search_pins WHERE id = $1 AND channel_id = $2;

-- name: ListSearchRedirects :many
SELECT * FROM search_redirects
WHERE  channel_id = $1
ORDER  BY query;

-- name: GetSearchRedirect :one
SELECT * FROM search_redirects
WHERE  channel_id = $1 AND query = $2;

-- name: UpsertSearchRedirect :one
INSERT INTO search_redirects (channel_id, query, target_url)
VALUES ($1, $2, $3)
ON CONFLICT (channel_id, query) DO UPDATE SET target_url = EXCLUDED.target_url
RETURNING *;

-- name: DeleteSearchRedirect :exec
DELETE FROM search_redirects WHERE id = $1 AND channel_id = $2;

-- name: ListSearchBoosts :many
SELECT * FROM search_boosts
WHERE channel_id = $1 AND (sqlc.arg(q)::text = '' OR query = sqlc.arg(q)::text)
ORDER BY query, weight DESC;

-- name: BoostsForQuery :many
SELECT product_id, weight FROM search_boosts
WHERE channel_id = $1 AND query = $2;

-- name: UpsertSearchBoost :one
INSERT INTO search_boosts (channel_id, query, product_id, weight)
VALUES ($1, $2, $3, $4)
ON CONFLICT (channel_id, query, product_id)
DO UPDATE SET weight = EXCLUDED.weight
RETURNING *;

-- name: DeleteSearchBoost :exec
DELETE FROM search_boosts WHERE id = $1 AND channel_id = $2;
