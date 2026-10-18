-- GDPR erasure scrubs in place, since the customer row stays in commerce as an anchor - a guest row with the same address goes too

-- name: ScrubErasedRosterEntry :exec
-- Stamped with core's erase time, so a sweep page read before the erasure can't restore the address
UPDATE customer_roster
SET email = '', first_name = '', phone = '', is_active = false,
    marketing_opted_in = false, sms_opted_in = false, consents = '{}'::jsonb,
    source_read_at = COALESCE(sqlc.narg(erased_at)::timestamptz, now())
WHERE customer_id = sqlc.arg(customer_id);

-- name: ScrubErasedFlowRuns :exec
UPDATE flow_runs
SET email = '', context = '{}'::jsonb, resume_at = NULL, updated_at = now(),
    status = CASE WHEN status IN ('active', 'waiting') THEN 'cancelled' ELSE status END
WHERE customer_id = sqlc.arg(customer_id)::uuid;

-- name: ScrubErasedGuestFlowRuns :exec
UPDATE flow_runs
SET email = '', context = '{}'::jsonb, resume_at = NULL, updated_at = now(),
    status = CASE WHEN status IN ('active', 'waiting') THEN 'cancelled' ELSE status END
WHERE customer_id IS NULL AND lower(email) = lower(sqlc.arg(email)::text);

-- name: ScrubErasedCampaignRecipients :exec
-- Sent rows stay as the campaign's history with the address gone, and a row still waiting to send is skipped
UPDATE email_campaign_recipients
SET email = 'redacted-' || id::text || '@anonymized.invalid', first_name = '',
    error = CASE WHEN status = 'pending' THEN 'customer erased' ELSE error END,
    status = CASE WHEN status = 'pending' THEN 'skipped' ELSE status END
WHERE customer_id = sqlc.arg(customer_id)::uuid;

-- name: ScrubErasedGuestCampaignRecipients :exec
UPDATE email_campaign_recipients
SET email = 'redacted-' || id::text || '@anonymized.invalid', first_name = '',
    error = CASE WHEN status = 'pending' THEN 'customer erased' ELSE error END,
    status = CASE WHEN status = 'pending' THEN 'skipped' ELSE status END
WHERE customer_id IS NULL AND lower(email) = lower(sqlc.arg(email)::text);

-- name: DetachErasedLensSessions :exec
UPDATE lens_sessions SET customer_id = NULL WHERE customer_id = sqlc.arg(customer_id)::uuid;

-- name: MarkCustomerErased :exec
INSERT INTO erased_customers (customer_id, erased_at)
VALUES (sqlc.arg(customer_id), COALESCE(sqlc.narg(erased_at)::timestamptz, now()))
ON CONFLICT (customer_id) DO NOTHING;

-- name: IsCustomerErased :one
SELECT EXISTS (SELECT 1 FROM erased_customers WHERE customer_id = sqlc.arg(customer_id))::bool;
