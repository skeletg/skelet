-- name: CreateLibraryEmail :one
INSERT INTO email_library (channel_id, name, subject, preheader, blocks, styles, group_id, slot)
VALUES (sqlc.arg(channel_id), sqlc.arg(name), sqlc.arg(subject), sqlc.arg(preheader), sqlc.arg(blocks), sqlc.arg(styles), sqlc.narg(group_id), sqlc.arg(slot))
RETURNING *;

-- name: GetLibraryEmail :one
SELECT * FROM email_library WHERE id = sqlc.arg(id);

-- name: ListLibraryEmails :many
-- Optional group filter ('' = all groups) - powers the unified emails list
SELECT * FROM email_library
WHERE channel_id = sqlc.arg(channel_id)
  AND (sqlc.arg(group_slug)::text = '' OR group_id = (SELECT id FROM email_groups WHERE slug = sqlc.arg(group_slug)::text))
ORDER BY name;

-- name: GetSlotEmail :one
-- The email assigned to a transactional slot for a channel (if any) - the render
-- path checks this before the email_templates -> built-in fallback
SELECT * FROM email_library WHERE channel_id = sqlc.arg(channel_id) AND slot = sqlc.arg(slot);

-- name: UpdateLibraryEmail :execrows
UPDATE email_library
SET name = sqlc.arg(name), subject = sqlc.arg(subject), preheader = sqlc.arg(preheader), blocks = sqlc.arg(blocks), styles = sqlc.arg(styles),
    group_id = sqlc.narg(group_id), updated_at = NOW()
WHERE id = sqlc.arg(id);

-- name: ClearLibraryEmailSlot :exec
-- Free a slot within a channel (used before assigning it to another email so
-- the (channel, slot) unique index holds - a slot has one email)
UPDATE email_library SET slot = '' WHERE channel_id = sqlc.arg(channel_id) AND slot = sqlc.arg(slot) AND slot <> '';

-- name: AssignLibraryEmailSlot :exec
UPDATE email_library SET slot = sqlc.arg(slot), updated_at = NOW() WHERE id = sqlc.arg(id);

-- name: DeleteLibraryEmail :exec
DELETE FROM email_library WHERE id = sqlc.arg(id);

-- --- email groups (admin taxonomy) ---

-- name: ListEmailGroups :many
SELECT * FROM email_groups ORDER BY sort_order, name;

-- name: GetEmailGroup :one
SELECT * FROM email_groups WHERE id = sqlc.arg(id);

-- name: CreateEmailGroup :one
INSERT INTO email_groups (slug, name, is_system, sort_order)
VALUES (sqlc.arg(slug), sqlc.arg(name), FALSE, sqlc.arg(sort_order))
RETURNING *;

-- name: RenameEmailGroup :one
UPDATE email_groups SET name = sqlc.arg(name) WHERE id = sqlc.arg(id) RETURNING *;

-- name: DeleteEmailGroup :execrows
-- Any group is deletable (incl. the seeded transactional/marketing ones);
-- email_library.group_id ON DELETE SET NULL keeps the emails, just untagged
DELETE FROM email_groups WHERE id = sqlc.arg(id);

-- name: CountEmailsInGroup :one
SELECT COUNT(*)::bigint FROM email_library WHERE group_id = sqlc.arg(group_id);

-- --- email version history ---

-- name: InsertEmailRevision :exec
INSERT INTO email_library_revisions (email_id, name, subject, preheader, blocks, styles, author_admin_id)
VALUES (sqlc.arg(email_id), sqlc.arg(name), sqlc.arg(subject), sqlc.arg(preheader), sqlc.arg(blocks), sqlc.arg(styles), sqlc.narg(author_admin_id));

-- name: ListEmailRevisions :many
SELECT id, email_id, name, subject, preheader, blocks, styles, author_admin_id, created_at
FROM email_library_revisions
WHERE email_id = sqlc.arg(email_id)
ORDER BY created_at DESC
LIMIT sqlc.arg(lim);

-- name: GetEmailRevision :one
SELECT id, email_id, name, subject, preheader, blocks, styles, author_admin_id, created_at
FROM email_library_revisions WHERE id = sqlc.arg(id);

-- name: PruneEmailRevisions :exec
-- Keep the newest `keep` revisions per email; drop older ones
DELETE FROM email_library_revisions r
WHERE r.email_id = sqlc.arg(email_id)
  AND r.id NOT IN (
    SELECT id FROM email_library_revisions
    WHERE email_id = sqlc.arg(email_id)
    ORDER BY created_at DESC
    LIMIT sqlc.arg(keep)
  );
