-- name: UpsertSignalsConfigValue :one
INSERT INTO config_values (scope, channel_id, path, value)
VALUES (sqlc.arg(scope), sqlc.narg(channel_id), sqlc.arg(path), sqlc.arg(value))
ON CONFLICT (scope, channel_id, path) DO UPDATE
  SET value = EXCLUDED.value, updated_at = NOW()
RETURNING *;

-- name: DeleteSignalsConfigValue :exec
DELETE FROM config_values
WHERE scope = sqlc.arg(scope)
  AND channel_id IS NOT DISTINCT FROM sqlc.narg(channel_id)
  AND path = sqlc.arg(path);

-- name: SignalsConfigCandidates :many
-- The rows a read has to consider: the platform default, plus the channel override when one is asked for
SELECT * FROM config_values
WHERE path = sqlc.arg(path)
  AND (
    (scope = 'default' AND channel_id IS NULL)
    OR (sqlc.arg(scope)::text = 'channel' AND scope = 'channel' AND channel_id = sqlc.narg(channel_id)::uuid)
  );

-- name: ListSignalsConfigValuesInScopes :many
SELECT * FROM config_values
WHERE (scope = 'default' AND channel_id IS NULL)
   OR (sqlc.arg(scope)::text = 'channel' AND scope = 'channel' AND channel_id = sqlc.narg(channel_id)::uuid);

-- name: ListAllSignalsConfigValues :many
SELECT * FROM config_values ORDER BY scope, channel_id, path;
