-- Boards: dashboards of saved reports, per shop - owned and private, shared with admins or roles, or shop boards. Every
-- read and write of a board names its shop, so an id alone reaches nothing in another

-- name: ListBoards :many
SELECT b.id, b.name, b.updated_at, b.owner_admin_id, (SELECT count(*) FROM board_reports r WHERE r.board_id = b.id)::int AS report_count
FROM boards b WHERE b.channel_id = sqlc.arg(channel_id) ORDER BY b.name, b.id;

-- name: GetBoard :one
SELECT id, name, updated_at, owner_admin_id FROM boards WHERE id = sqlc.arg(id) AND channel_id = sqlc.arg(channel_id);

-- name: BoardReports :many
SELECT title, size, definition FROM board_reports WHERE board_id = sqlc.arg(board_id) ORDER BY position;

-- name: CreateBoard :one
INSERT INTO boards (channel_id, name, owner_admin_id)
VALUES (sqlc.arg(channel_id), sqlc.arg(name), sqlc.narg(owner_admin_id)) RETURNING id, updated_at;

-- name: TouchBoard :execrows
-- Only when nobody saved it since the caller read it - a stale save would silently drop the other admin's reports
UPDATE boards SET name = sqlc.arg(name), updated_at = now()
WHERE id = sqlc.arg(id) AND channel_id = sqlc.arg(channel_id) AND updated_at = sqlc.arg(read_at);

-- name: ClearBoardReports :exec
DELETE FROM board_reports WHERE board_id = sqlc.arg(board_id);

-- name: AddBoardReport :exec
INSERT INTO board_reports (board_id, position, title, size, definition)
VALUES (sqlc.arg(board_id), sqlc.arg(position), sqlc.arg(title), sqlc.arg(size), sqlc.arg(definition));

-- name: DeleteBoard :execrows
DELETE FROM boards WHERE id = sqlc.arg(id) AND channel_id = sqlc.arg(channel_id);

-- name: PruneOrphanedBoards :execrows
DELETE FROM boards b WHERE NOT EXISTS (SELECT 1 FROM channel_roster c WHERE c.channel_id = b.channel_id);

-- name: LockChannelBoards :exec
-- Serialises the starter boards per shop, so two admins clicking at once can't both see an empty shop
SELECT pg_advisory_xact_lock(hashtextextended('boards-starters:' || sqlc.arg(channel_id)::uuid::text, 0));

-- name: CountBoards :one
SELECT count(*)::int FROM boards WHERE channel_id = sqlc.arg(channel_id);

-- name: SetBoardOwner :execrows
-- NULL makes it a shop board
UPDATE boards SET owner_admin_id = sqlc.narg(owner_admin_id), updated_at = now() WHERE id = sqlc.arg(id) AND channel_id = sqlc.arg(channel_id);

-- name: BoardShares :many
SELECT admin_id, role_id, level FROM board_shares WHERE board_id = sqlc.arg(board_id) ORDER BY created_at, admin_id, role_id;

-- name: ChannelBoardShares :many
-- Every share on a shop's boards, so a list works out each board's access in one read
SELECT s.board_id, s.admin_id, s.role_id, s.level FROM board_shares s JOIN boards b ON b.id = s.board_id
WHERE b.channel_id = sqlc.arg(channel_id);

-- name: RemoveBoardAdminShare :exec
-- The new owner of a board owns it - a share of their own would only shadow that
DELETE FROM board_shares WHERE board_id = sqlc.arg(board_id) AND admin_id = sqlc.arg(admin_id);

-- name: ClearBoardShares :exec
DELETE FROM board_shares WHERE board_id = sqlc.arg(board_id);

-- name: AddBoardShare :exec
INSERT INTO board_shares (board_id, admin_id, role_id, level, created_by)
VALUES (sqlc.arg(board_id), sqlc.narg(admin_id), sqlc.narg(role_id), sqlc.arg(level), sqlc.arg(created_by));

-- name: AdminFavouriteBoards :many
SELECT board_id FROM board_favourites WHERE admin_id = sqlc.arg(admin_id);

-- name: AddBoardFavourite :exec
INSERT INTO board_favourites (admin_id, board_id) VALUES (sqlc.arg(admin_id), sqlc.arg(board_id)) ON CONFLICT DO NOTHING;

-- name: RemoveBoardFavourite :exec
DELETE FROM board_favourites WHERE admin_id = sqlc.arg(admin_id) AND board_id = sqlc.arg(board_id);

-- name: AdminDraftBoards :many
SELECT board_id, updated_at FROM board_drafts WHERE admin_id = sqlc.arg(admin_id);

-- name: GetBoardDraft :one
SELECT payload, base_updated_at, updated_at FROM board_drafts WHERE board_id = sqlc.arg(board_id) AND admin_id = sqlc.arg(admin_id);

-- name: PutBoardDraft :one
INSERT INTO board_drafts (board_id, admin_id, payload, base_updated_at) VALUES (sqlc.arg(board_id), sqlc.arg(admin_id), sqlc.arg(payload), sqlc.arg(base_updated_at))
ON CONFLICT (board_id, admin_id) DO UPDATE SET payload = EXCLUDED.payload, base_updated_at = EXCLUDED.base_updated_at, updated_at = now()
RETURNING updated_at;

-- name: DeleteBoardDraft :exec
DELETE FROM board_drafts WHERE board_id = sqlc.arg(board_id) AND admin_id = sqlc.arg(admin_id);

-- name: SweepBoardDrafts :execrows
-- Unsaved edits nobody came back to - a draft board itself stays, only its owner sees it
DELETE FROM board_drafts WHERE updated_at < sqlc.arg(before);
