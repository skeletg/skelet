-- The email send log, written over the contract as core's campaigns and flows send
-- Append-only, plus the human-only first-open and first-click stamps the open and click handlers maintain

-- name: InsertEmailSend :one
-- The id is the caller's: core mints it before the mail goes out, so the tracking token it injects
-- names a row that exists by the time a mail client loads the pixel
INSERT INTO email_sends (
    id, channel_id, source, customer_id, email,
    campaign_id, recipient_id, variant, flow_id, flow_run_id, flow_node_id, message_key, order_id
) VALUES (
    sqlc.arg(id), sqlc.arg(channel_id), sqlc.arg(source), sqlc.narg(customer_id), sqlc.arg(email),
    sqlc.narg(campaign_id), sqlc.narg(recipient_id), sqlc.arg(variant), sqlc.narg(flow_id),
    sqlc.narg(flow_run_id), sqlc.arg(flow_node_id), sqlc.arg(message_key), sqlc.narg(order_id)
)
RETURNING id, sent_at;

-- name: GetEmailSend :one
-- The pixel/click hot path. The 90-day bound exists for partition pruning - the table is monthly-partitioned on sent_at and an id-only probe would touch every partition forever. An older send just stops recording engagement (click redirects never break: the destination URL rides the token, not this row).
SELECT * FROM email_sends
WHERE id = sqlc.arg(id) AND sent_at >= now() - interval '90 days';

-- name: MarkSendOpened :exec
UPDATE email_sends
SET first_open_at = COALESCE(first_open_at, NOW())
WHERE id = sqlc.arg(id) AND sent_at >= now() - interval '90 days';

-- name: MarkSendClicked :exec
UPDATE email_sends
SET first_click_at = COALESCE(first_click_at, NOW())
WHERE id = sqlc.arg(id) AND sent_at >= now() - interval '90 days';

-- name: EnsureEmailSendPartitions :exec
-- Pre-creates the next monthly partitions so inserts never land in the DEFAULT safety net (a populated DEFAULT blocks creating the month it holds). Rides the daily analytics prune.
SELECT ensure_email_send_partitions(3);

-- name: EmailSendMonthsBefore :many
-- The send-log retention sweep's work list: months (partitions + any
-- default-partition strays) lying entirely before the cutoff.
SELECT months.month::date FROM email_send_months_before(sqlc.arg(cutoff)) AS months(month);

-- name: DropEmailSendMonth :one
-- Drops one whole month of sends (partition drop + stray delete), returning the rows removed. Only called for months the archive export has committed, or directly under retention_action=prune.
SELECT drop_email_send_month(sqlc.arg(month)) AS dropped;

-- name: ListEmailSendsForArchive :many
-- One keyset page of a month for the archive export, ordered by id (uuidv7, so the (id, sent_at) primary key walks it in near-time order). customer_id, email and recipient_id are deliberately not selected - archives are identity-stripped, keeping campaign/flow/order linkage for aggregate stats.
SELECT id, channel_id, source, campaign_id, variant, flow_id, flow_run_id,
       flow_node_id, message_key, order_id, sent_at, first_open_at, first_click_at
FROM email_sends
WHERE sent_at >= sqlc.arg(from_at) AND sent_at < sqlc.arg(to_at)
  AND id > sqlc.arg(cursor_id)
ORDER BY id
LIMIT sqlc.arg(lim);

-- name: ScrubCustomerEmailSends :execrows
-- GDPR erasure: redact the address on the customer's sends. Matched by customer id plus the pre-scrub address itself, which also catches guest-era sends to the same person. The rows stay (the send happened), like the order scrub.
UPDATE email_sends
SET email = 'redacted-' || sqlc.arg(customer_id)::uuid::text || '@anonymized.invalid'
WHERE customer_id = sqlc.arg(customer_id)::uuid
   OR email = sqlc.arg(email)::text;

-- name: CampaignVariantStats :many
-- Per-variant sent / opened / clicked (human, bot-filtered) for the A/B results view and the winner decision. Groups on the denormalized variant, which is cheaper than joining email_campaign_recipients.
SELECT
    variant                                                   AS variant,
    COUNT(*)::int                                             AS sent,
    COUNT(*) FILTER (WHERE first_open_at IS NOT NULL)::int     AS opened,
    COUNT(*) FILTER (WHERE first_click_at IS NOT NULL)::int    AS clicked
FROM email_sends
WHERE campaign_id = sqlc.arg(campaign_id) AND variant <> ''
GROUP BY variant
ORDER BY variant;
