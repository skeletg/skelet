-- name: CreateEmailCampaign :one
INSERT INTO email_campaigns (channel_id, name, subject, blocks, styles, audience_kind, audience_id, email_id,
    ab_enabled, ab_subject_b, ab_test_percent, ab_winner_metric, ab_test_minutes)
VALUES (sqlc.arg(channel_id), sqlc.arg(name), sqlc.arg(subject), sqlc.arg(blocks), sqlc.arg(styles), sqlc.arg(audience_kind), sqlc.narg(audience_id), sqlc.narg(email_id),
    sqlc.arg(ab_enabled), sqlc.arg(ab_subject_b), sqlc.arg(ab_test_percent), sqlc.arg(ab_winner_metric), sqlc.arg(ab_test_minutes))
RETURNING *;

-- name: GetEmailCampaign :one
SELECT * FROM email_campaigns WHERE id = sqlc.arg(id);

-- name: ListEmailCampaigns :many
SELECT * FROM email_campaigns
WHERE channel_id = sqlc.arg(channel_id)
  AND (sqlc.arg(include_deleted)::bool OR deleted_at IS NULL)
ORDER BY created_at DESC;

-- name: UpdateEmailCampaign :execrows
-- Only drafts are editable
UPDATE email_campaigns
SET name = sqlc.arg(name), subject = sqlc.arg(subject), blocks = sqlc.arg(blocks),
    styles = sqlc.arg(styles), audience_kind = sqlc.arg(audience_kind),
    audience_id = sqlc.narg(audience_id), email_id = sqlc.narg(email_id),
    ab_enabled = sqlc.arg(ab_enabled), ab_subject_b = sqlc.arg(ab_subject_b),
    ab_test_percent = sqlc.arg(ab_test_percent), ab_winner_metric = sqlc.arg(ab_winner_metric),
    ab_test_minutes = sqlc.arg(ab_test_minutes), updated_at = NOW()
WHERE id = sqlc.arg(id) AND status = 'draft';

-- name: ScheduleEmailCampaign :execrows
-- Move a draft (or an already-scheduled campaign) to scheduled with a send time
UPDATE email_campaigns
SET status = 'scheduled', scheduled_for = sqlc.arg(scheduled_for), updated_at = NOW()
WHERE id = sqlc.arg(id) AND status IN ('draft', 'scheduled');

-- name: CancelEmailCampaign :execrows
UPDATE email_campaigns
SET status = 'cancelled', updated_at = NOW()
WHERE id = sqlc.arg(id) AND status IN ('scheduled', 'sending', 'testing');

-- name: SoftDeleteEmailCampaign :execrows
-- Soft delete: a sent campaign keeps its recipients, opens and clicks
UPDATE email_campaigns SET deleted_at = NOW(), updated_at = NOW()
WHERE id = sqlc.arg(id) AND deleted_at IS NULL;

-- name: RestoreEmailCampaign :execrows
UPDATE email_campaigns SET deleted_at = NULL, updated_at = NOW()
WHERE id = sqlc.arg(id) AND deleted_at IS NOT NULL;

-- name: ClaimDueCampaigns :many
-- Atomically lease due scheduled campaigns for sending (multi-replica safe)
UPDATE email_campaigns
SET status = 'sending', updated_at = NOW()
WHERE id IN (
    SELECT id FROM email_campaigns
    WHERE status = 'scheduled' AND scheduled_for <= NOW() AND deleted_at IS NULL
    ORDER BY scheduled_for
    LIMIT sqlc.arg('lim')
    FOR UPDATE SKIP LOCKED
)
RETURNING *;

-- name: ListSendingCampaigns :many
-- Least recently advanced first, since each pass touches updated_at - a half-written audience never sends
SELECT * FROM email_campaigns WHERE status = 'sending' AND deleted_at IS NULL AND materialized_at IS NOT NULL ORDER BY updated_at, scheduled_for;

-- name: ListUnmaterializedCampaigns :many
-- Claimed but not yet fully materialized, retried until complete
SELECT * FROM email_campaigns WHERE status = 'sending' AND deleted_at IS NULL AND materialized_at IS NULL ORDER BY updated_at;

-- name: InsertCampaignRecipients :exec
-- Re-inserting a recipient is a no-op, so a restarted materialization writes the whole audience again
INSERT INTO email_campaign_recipients (campaign_id, customer_id, email, first_name, variant, phase)
SELECT sqlc.arg(campaign_id), unnest(sqlc.arg(customer_ids)::uuid[]), unnest(sqlc.arg(emails)::text[]), unnest(sqlc.arg(first_names)::text[]), unnest(sqlc.arg(variants)::text[]), unnest(sqlc.arg(phases)::text[])
ON CONFLICT (campaign_id, email) DO NOTHING;

-- name: MarkCampaignMaterialized :exec
-- Written only after every batch landed
UPDATE email_campaigns c
SET total_recipients = (SELECT COUNT(*) FROM email_campaign_recipients r WHERE r.campaign_id = c.id),
    materialized_at = NOW(), updated_at = NOW()
WHERE c.id = sqlc.arg(id);

-- name: ListPendingRecipients :many
-- Holdout recipients (awaiting the A/B winner) are excluded - they're sent only
-- after the winner is assigned (phase flips to 'send')
SELECT * FROM email_campaign_recipients
WHERE campaign_id = sqlc.arg(campaign_id) AND status = 'pending' AND phase <> 'holdout'
ORDER BY id
LIMIT sqlc.arg('lim');

-- name: HasPendingNonHoldoutRecipients :one
SELECT EXISTS (SELECT 1 FROM email_campaign_recipients
WHERE campaign_id = sqlc.arg(campaign_id) AND status = 'pending' AND phase <> 'holdout');

-- name: HasPendingRecipients :one
SELECT EXISTS (SELECT 1 FROM email_campaign_recipients
WHERE campaign_id = sqlc.arg(campaign_id) AND status = 'pending');

-- name: MarkRecipientSent :execrows
-- Claimed BEFORE the message is handed to the sender: this row is the only thing that stops a resend, so marking after delivery leaves a window where a failed write re-mails the recipient on every later tick - returns rows affected so the caller can tell "I own this one" from "a concurrent sweep already took it"
-- The campaign must still be sending, so a cancel or delete stops the pass already in flight at its next recipient
UPDATE email_campaign_recipients r
SET status = 'sent', sent_at = NOW()
WHERE r.id = sqlc.arg(id) AND r.status = 'pending'
  AND EXISTS (SELECT 1 FROM email_campaigns c WHERE c.id = r.campaign_id AND c.status = 'sending' AND c.deleted_at IS NULL);

-- name: MarkRecipientSendFailed :exec
-- Releases a claim when the send itself fails - guarded on 'sent' because only the sweep holding the claim may flip it, and the recipient keeps its failure reason rather than silently reading as delivered
UPDATE email_campaign_recipients
SET status = 'failed', error = sqlc.arg(error), sent_at = NULL
WHERE id = sqlc.arg(id) AND status = 'sent';

-- name: ReleaseRecipientClaim :exec
-- Hands a claim back untouched when the send was abandoned before any mail went out, so the next tick retries the recipient
UPDATE email_campaign_recipients
SET status = 'pending', sent_at = NULL
WHERE id = sqlc.arg(id) AND status = 'sent';

-- name: MarkRecipientFailed :exec
UPDATE email_campaign_recipients
SET status = 'failed', error = sqlc.arg(error)
WHERE id = sqlc.arg(id) AND status = 'pending';

-- name: MarkRecipientSkipped :exec
-- Suppressed at send time (e.g. customer not opted in to marketing email)
UPDATE email_campaign_recipients
SET status = 'skipped', error = sqlc.arg(error)
WHERE id = sqlc.arg(id) AND status = 'pending';

-- name: AddCampaignCounts :exec
-- One pass's outcomes - RefreshCampaignCounts squares the totals at state changes
UPDATE email_campaigns
SET sent_count = sent_count + sqlc.arg(sent)::int, failed_count = failed_count + sqlc.arg(failed)::int,
    skipped_count = skipped_count + sqlc.arg(skipped)::int, updated_at = NOW()
WHERE id = sqlc.arg(id);

-- name: RefreshCampaignCounts :exec
UPDATE email_campaigns c
SET sent_count = (SELECT COUNT(*) FROM email_campaign_recipients r WHERE r.campaign_id = c.id AND r.status = 'sent'),
    failed_count = (SELECT COUNT(*) FROM email_campaign_recipients r WHERE r.campaign_id = c.id AND r.status = 'failed'),
    skipped_count = (SELECT COUNT(*) FROM email_campaign_recipients r WHERE r.campaign_id = c.id AND r.status = 'skipped'),
    updated_at = NOW()
WHERE c.id = sqlc.arg(id);

-- name: FinalizeCampaign :exec
-- Only from sending - a campaign cancelled mid-pass stays cancelled
UPDATE email_campaigns
SET status = 'sent', sent_at = NOW(), updated_at = NOW()
WHERE id = sqlc.arg(id) AND status = 'sending';

-- name: RecordRecipientOpen :execrows
-- Sets the first-open timestamp; returns 1 only on the first open (dedupe)
UPDATE email_campaign_recipients
SET opened_at = NOW()
WHERE id = sqlc.arg(id) AND opened_at IS NULL;

-- name: RecordRecipientClick :execrows
UPDATE email_campaign_recipients
SET clicked_at = NOW()
WHERE id = sqlc.arg(id) AND clicked_at IS NULL;

-- name: BumpCampaignOpened :exec
UPDATE email_campaigns SET opened_count = opened_count + 1, updated_at = NOW() WHERE id = sqlc.arg(id);

-- name: BumpCampaignClicked :exec
UPDATE email_campaigns SET clicked_count = clicked_count + 1, updated_at = NOW() WHERE id = sqlc.arg(id);

-- --- A/B subject test ---

-- name: MarkCampaignTesting :execrows
-- Test slice fully sent → enter the measurement window (sending → testing)
UPDATE email_campaigns SET status = 'testing', ab_test_started_at = NOW(), updated_at = NOW()
WHERE id = sqlc.arg(id) AND status = 'sending';

-- name: ListABTestingDue :many
-- A/B campaigns whose measurement window has elapsed - ready to pick a winner
SELECT * FROM email_campaigns
WHERE status = 'testing' AND deleted_at IS NULL AND ab_test_started_at IS NOT NULL
  AND ab_test_started_at + make_interval(mins => ab_test_minutes) <= NOW()
ORDER BY ab_test_started_at;

-- name: AssignHoldoutToWinner :exec
-- Send the winning variant to the held-back remainder (phase flips to 'send')
UPDATE email_campaign_recipients SET variant = sqlc.arg(variant), phase = 'send'
WHERE campaign_id = sqlc.arg(campaign_id) AND phase = 'holdout';

-- name: SetCampaignWinner :execrows
-- Record the winner and resume sending (testing → sending) so the holdout goes out
UPDATE email_campaigns SET ab_winner = sqlc.arg(winner), status = 'sending', updated_at = NOW()
WHERE id = sqlc.arg(id) AND status = 'testing';

-- name: ListCampaignsWithRecipientsSentBefore :many
-- Sent campaigns past the send-history window that still carry recipient rows - their counts live on the campaign row
SELECT c.id FROM email_campaigns c
WHERE ((c.status = 'sent' AND c.sent_at < sqlc.arg(before)) OR (c.status = 'cancelled' AND c.updated_at < sqlc.arg(before)))
  AND EXISTS (SELECT 1 FROM email_campaign_recipients r WHERE r.campaign_id = c.id);

-- name: DeleteCampaignRecipients :execrows
DELETE FROM email_campaign_recipients WHERE campaign_id = sqlc.arg(campaign_id);
