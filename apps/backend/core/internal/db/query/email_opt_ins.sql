-- name: RecordEmailOptIn :exec
-- A repeat tick refreshes the proof to the latest one
INSERT INTO email_opt_ins (channel_id, email, kind, source, reference)
VALUES (sqlc.arg(channel_id), sqlc.arg(email), sqlc.arg(kind), sqlc.arg(source), sqlc.arg(reference))
ON CONFLICT (channel_id, email, kind) DO UPDATE
SET source = EXCLUDED.source, reference = EXCLUDED.reference, created_at = NOW();

-- name: HasEmailOptIn :one
-- A guest's own permission for their address - never read as an account's consent
SELECT EXISTS (
    SELECT 1 FROM email_opt_ins
    WHERE  channel_id = sqlc.arg(channel_id) AND email = sqlc.arg(email) AND kind = sqlc.arg(kind)
);
