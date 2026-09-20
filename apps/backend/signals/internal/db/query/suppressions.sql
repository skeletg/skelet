-- Email suppression list - address-level, global - the send path calls IsEmailSuppressed before every marketing/transactional send; the
-- deliverability ingest path records bounces/complaints; the dashboard lists/adds/removes

-- name: IsEmailSuppressed :one
-- Hot path: one indexed lookup before every send - true = do not send
SELECT EXISTS (
    SELECT 1 FROM email_suppressions WHERE email = sqlc.arg(email) AND active = TRUE
);

-- name: ListActiveSuppressedEmails :many
-- Batch variant of IsEmailSuppressed: returns which of the given emails are actively suppressed, so a send batch checks the whole recipient set in one query instead of one lookup per address
SELECT email FROM email_suppressions
WHERE active = TRUE AND email = ANY(sqlc.arg(emails)::text[]);

-- name: SuppressEmail :exec
-- Suppress immediately (hard bounce / complaint / manual / unsubscribe) - idempotent:
-- re-suppressing refreshes the reason/detail and keeps active true
INSERT INTO email_suppressions (email, reason, detail, active)
VALUES (sqlc.arg(email), sqlc.arg(reason), sqlc.arg(detail), TRUE)
ON CONFLICT (email) DO UPDATE
    SET reason = excluded.reason, detail = excluded.detail, active = TRUE, updated_at = NOW();

-- name: RecordSoftBounce :one
-- Accumulate a soft bounce; suppress once the tally reaches the threshold - the caller passes the configured threshold - returns the row so the caller can tell whether this bounce tripped suppression (for a notification)
INSERT INTO email_suppressions (email, reason, detail, soft_bounces, active)
VALUES (sqlc.arg(email), 'soft_bounce', sqlc.arg(detail), 1, 1 >= sqlc.arg(threshold)::int)
ON CONFLICT (email) DO UPDATE
    SET soft_bounces = email_suppressions.soft_bounces + 1,
        detail       = excluded.detail,
        -- A prior hard suppression stays active; otherwise trip at threshold
        active       = email_suppressions.active OR (email_suppressions.soft_bounces + 1 >= sqlc.arg(threshold)::int),
        reason       = CASE WHEN email_suppressions.active THEN email_suppressions.reason ELSE 'soft_bounce' END,
        updated_at   = NOW()
RETURNING active, soft_bounces;

-- name: UnsuppressEmail :exec
-- Operator clears a suppression (address recovered / added in error) - resets the
-- soft-bounce tally so it starts fresh
UPDATE email_suppressions
SET active = FALSE, soft_bounces = 0, updated_at = NOW()
WHERE email = sqlc.arg(email);

-- name: ListSuppressions :many
-- Dashboard list, newest first, optional reason filter ('' = all) - active only
SELECT id, email, reason, detail, soft_bounces, created_at, updated_at
FROM email_suppressions
WHERE active = TRUE
  AND (sqlc.arg(reason)::text = '' OR reason = sqlc.arg(reason)::text)
ORDER BY created_at DESC
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: CountSuppressions :one
SELECT COUNT(*)::bigint FROM email_suppressions
WHERE active = TRUE AND (sqlc.arg(reason)::text = '' OR reason = sqlc.arg(reason)::text);
