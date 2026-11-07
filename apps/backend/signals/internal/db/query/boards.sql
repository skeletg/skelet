-- Boards: shared dashboards of saved reports, per shop. Every read and write names the board's shop, so an id alone
-- reaches nothing in another

-- name: ListBoards :many
SELECT b.id, b.name, b.updated_at, (SELECT count(*) FROM board_reports r WHERE r.board_id = b.id)::int AS report_count
FROM boards b WHERE b.channel_id = sqlc.arg(channel_id) ORDER BY b.name, b.id;

-- name: GetBoard :one
SELECT id, name, updated_at FROM boards WHERE id = sqlc.arg(id) AND channel_id = sqlc.arg(channel_id);

-- name: BoardReports :many
SELECT title, size, definition FROM board_reports WHERE board_id = sqlc.arg(board_id) ORDER BY position;

-- name: CreateBoard :one
INSERT INTO boards (channel_id, name) VALUES (sqlc.arg(channel_id), sqlc.arg(name)) RETURNING id, updated_at;

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
