-- name: GetEmailTemplate :one
-- Fetch one template by key + scope - pass channel_id NULL for the global row,
-- a uuid for a channel override - IS NOT DISTINCT FROM matches NULL to NULL
SELECT * FROM email_templates
WHERE key = sqlc.arg(key)
  AND channel_id IS NOT DISTINCT FROM sqlc.narg(channel_id);

-- name: GetEmailBlock :one
SELECT * FROM email_blocks WHERE channel_id = sqlc.arg(channel_id) AND slug = sqlc.arg(slug);

-- name: ListEmailBlocks :many
SELECT * FROM email_blocks WHERE channel_id = sqlc.arg(channel_id) ORDER BY name, slug;

-- name: UpsertEmailBlock :one
INSERT INTO email_blocks (channel_id, slug, name, icon, fields, html_template, text_template, css, scripted, blocks)
VALUES (sqlc.arg(channel_id), sqlc.arg(slug), sqlc.arg(name), sqlc.arg(icon), sqlc.arg(fields), sqlc.arg(html_template), sqlc.arg(text_template), sqlc.arg(css), sqlc.arg(scripted), sqlc.arg(blocks))
ON CONFLICT (channel_id, slug) DO UPDATE
SET name = EXCLUDED.name, icon = EXCLUDED.icon, fields = EXCLUDED.fields,
    html_template = EXCLUDED.html_template, text_template = EXCLUDED.text_template,
    css = EXCLUDED.css, scripted = EXCLUDED.scripted,
    blocks = EXCLUDED.blocks, updated_at = NOW()
RETURNING *;

-- name: DeleteEmailBlock :exec
DELETE FROM email_blocks WHERE channel_id = sqlc.arg(channel_id) AND slug = sqlc.arg(slug);
