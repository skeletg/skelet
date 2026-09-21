-- Custom web forms + submissions

-- name: CreateWebForm :one
INSERT INTO web_forms (channel_id, slug, name, status, fields, settings)
VALUES (sqlc.arg(channel_id), sqlc.arg(slug), sqlc.arg(name), sqlc.arg(status), sqlc.arg(fields), sqlc.arg(settings))
RETURNING *;

-- name: UpdateWebForm :one
UPDATE web_forms SET
    name = sqlc.arg(name), status = sqlc.arg(status),
    fields = sqlc.arg(fields), settings = sqlc.arg(settings), updated_at = NOW()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: GetWebForm :one
SELECT * FROM web_forms WHERE id = sqlc.arg(id);

-- name: GetWebFormBySlug :one
-- Storefront lookup - an archived form must stop rendering and stop
-- accepting submissions, so it is filtered here rather than at the edge
SELECT * FROM web_forms
WHERE channel_id = sqlc.arg(channel_id) AND slug = sqlc.arg(slug) AND deleted_at IS NULL;

-- name: SoftDeleteWebForm :execrows
-- Soft delete: submissions already collected stay attached to a real form
UPDATE web_forms SET deleted_at = NOW(), updated_at = NOW()
WHERE id = sqlc.arg(id) AND deleted_at IS NULL;

-- name: RestoreWebForm :execrows
UPDATE web_forms SET deleted_at = NULL, updated_at = NOW()
WHERE id = sqlc.arg(id) AND deleted_at IS NOT NULL;

-- name: ListWebForms :many
-- Forms for a channel (dashboard picker + list), newest first
SELECT * FROM web_forms
WHERE channel_id = sqlc.arg(channel_id)
  AND (sqlc.arg(include_deleted)::bool OR deleted_at IS NULL)
ORDER BY created_at DESC;

-- name: RecordWebFormSubmission :one
INSERT INTO web_form_submissions (form_id, channel_id, answers, files, customer_id, meta)
VALUES (sqlc.arg(form_id), sqlc.arg(channel_id), sqlc.arg(answers), sqlc.arg(files), sqlc.narg(customer_id), sqlc.arg(meta))
RETURNING *;

-- name: GetWebFormSubmission :one
SELECT * FROM web_form_submissions WHERE id = sqlc.arg(id);

-- name: CountWebFormSubmissions :one
SELECT COUNT(*)::bigint FROM web_form_submissions WHERE form_id = sqlc.arg(form_id);

-- name: ListWebFormSubmissions :many
SELECT * FROM web_form_submissions
WHERE form_id = sqlc.arg(form_id)
ORDER BY created_at DESC
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);
