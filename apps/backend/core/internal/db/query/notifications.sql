-- Admin inbox queries - visibility filter is shared across list/count/mark-all: a notification is visible to an admin when (a) they bypass scope (superuser / all-channel), OR (b) they hold its required_permission AND it's global (channel_id IS NULL) or in one of their permitted channels On top of that, categories the admin muted are excluded from all three - the badge is counted here, so a mute the count didn't honour would be a badge that lies COALESCE, because an empty mute list arrives as NULL and `= ANY(NULL)` is NULL, so NOT of it drops every row - "nothing muted" would have emptied the whole inbox

-- name: CreateNotification :one
-- Idempotent: dedupe_key collapses repeat events / re-scans of the same condition - RETURNING feeds the live-push fan-out; a dedupe hit returns no row (sql.ErrNoRows), which the service treats as "already notified"
INSERT INTO admin_notifications (
    category, severity, title, body, required_permission, channel_id,
    entity_type, entity_id, action_url, dedupe_key
) VALUES (
    sqlc.arg(category), sqlc.arg(severity), sqlc.arg(title), sqlc.arg(body),
    sqlc.arg(required_permission), sqlc.narg(channel_id),
    sqlc.arg(entity_type), sqlc.arg(entity_id), sqlc.arg(action_url), sqlc.arg(dedupe_key)
)
ON CONFLICT (dedupe_key) DO NOTHING
RETURNING id, created_at;

-- name: ListNotifications :many
SELECT n.*, (r.read_at IS NOT NULL) AS is_read
FROM admin_notifications n
LEFT JOIN notification_reads r ON r.notification_id = n.id AND r.admin_id = sqlc.arg(admin_id)
WHERE (sqlc.arg(bypass)::bool OR n.required_permission = ANY(sqlc.arg(perms)::text[]))
  AND (sqlc.arg(bypass)::bool OR n.channel_id IS NULL OR n.channel_id = ANY(sqlc.arg(channels)::uuid[]))
  AND NOT (n.category = ANY(COALESCE(sqlc.arg(muted)::text[], '{}')))
  AND (NOT sqlc.arg(unread_only)::bool OR r.read_at IS NULL)
ORDER BY n.created_at DESC
LIMIT sqlc.arg(lim);

-- name: CountUnreadNotifications :one
SELECT COUNT(*)::bigint
FROM admin_notifications n
LEFT JOIN notification_reads r ON r.notification_id = n.id AND r.admin_id = sqlc.arg(admin_id)
WHERE (sqlc.arg(bypass)::bool OR n.required_permission = ANY(sqlc.arg(perms)::text[]))
  AND (sqlc.arg(bypass)::bool OR n.channel_id IS NULL OR n.channel_id = ANY(sqlc.arg(channels)::uuid[]))
  AND NOT (n.category = ANY(COALESCE(sqlc.arg(muted)::text[], '{}')))
  AND r.read_at IS NULL;

-- name: MarkNotificationRead :exec
INSERT INTO notification_reads (notification_id, admin_id)
VALUES (sqlc.arg(notification_id), sqlc.arg(admin_id))
ON CONFLICT DO NOTHING;

-- name: MarkAllNotificationsRead :exec
INSERT INTO notification_reads (notification_id, admin_id)
SELECT n.id, sqlc.arg(admin_id)
FROM admin_notifications n
LEFT JOIN notification_reads r ON r.notification_id = n.id AND r.admin_id = sqlc.arg(admin_id)
WHERE (sqlc.arg(bypass)::bool OR n.required_permission = ANY(sqlc.arg(perms)::text[]))
  AND (sqlc.arg(bypass)::bool OR n.channel_id IS NULL OR n.channel_id = ANY(sqlc.arg(channels)::uuid[]))
  AND NOT (n.category = ANY(COALESCE(sqlc.arg(muted)::text[], '{}')))
  AND r.read_at IS NULL
ON CONFLICT DO NOTHING;

-- name: ListFailingScheduledJobs :many
-- Standing-condition source for the inbox scan: enabled jobs whose last run errored
SELECT name, last_error FROM scheduled_jobs WHERE enabled AND last_status = 'error';

-- name: PruneNotifications :execrows
-- Retention: drop read notifications older than the cutoff (the daily
-- notifications-prune job)
DELETE FROM admin_notifications n
USING notification_reads r
WHERE r.notification_id = n.id AND n.created_at < sqlc.arg(cutoff);

-- name: PruneAllNotifications :execrows
-- Backstop at a longer window: an inbox nobody reads must not grow forever
-- notification_reads cascades on delete
DELETE FROM admin_notifications WHERE created_at < sqlc.arg(cutoff);
