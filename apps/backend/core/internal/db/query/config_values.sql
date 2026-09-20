-- name: UpsertConfigValue :one
INSERT INTO config_values (scope, channel_id, locale, path, value)
VALUES (sqlc.arg(scope), sqlc.narg(channel_id), sqlc.narg(locale), sqlc.arg(path), sqlc.arg(value))
ON CONFLICT (scope, channel_id, COALESCE(locale, ''), path) DO UPDATE
  SET value = EXCLUDED.value, updated_at = NOW()
RETURNING *;

-- name: DeleteConfigValue :exec
DELETE FROM config_values
WHERE scope = $1
  AND channel_id IS NOT DISTINCT FROM sqlc.narg(channel_id)
  AND locale IS NOT DISTINCT FROM sqlc.narg(locale)
  AND path = sqlc.arg(path);

-- name: GetConfigValueWithFallback :many
-- Returns rows needed for fallback resolution - for a locale-specific store view scope this can yield up to 3 rows: exact channel+locale, channel default (NULL locale), and default scope
SELECT * FROM config_values
WHERE path = $1
  AND (
    (scope = 'default' AND channel_id IS NULL AND locale IS NULL)
    OR (
      sqlc.arg(scope)::text = 'channel'
      AND scope = 'channel'
      AND channel_id = sqlc.narg(channel_id)::uuid
      AND (
        locale IS NULL
        OR (sqlc.narg(locale)::text IS NOT NULL AND locale = sqlc.narg(locale)::text)
      )
    )
  );

-- name: ListConfigValuesInScopes :many
-- For ListAll: every row at default plus any channel overrides needed for
-- fallback resolution - locale-specific listing includes channel defaults
SELECT * FROM config_values
WHERE (scope = 'default' AND channel_id IS NULL AND locale IS NULL)
   OR (
     sqlc.arg(scope)::text = 'channel'
     AND scope = 'channel'
     AND channel_id = sqlc.narg(channel_id)::uuid
     AND (
       locale IS NULL
       OR (sqlc.narg(locale)::text IS NOT NULL AND locale = sqlc.narg(locale)::text)
     )
   );

-- name: ListAllConfigValues :many
-- Full table dump for the CLI orphan detector
SELECT * FROM config_values ORDER BY scope, channel_id, locale, path;
