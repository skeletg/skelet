-- Signup forms + submissions

-- name: CreateSignupForm :one
INSERT INTO signup_forms (
    channel_id, slug, name, status, form_type, heading, body, button_label,
    success_message, collect_name, consent_kind, target_segment_id, trigger_flow, display_config
) VALUES (
    sqlc.arg(channel_id), sqlc.arg(slug), sqlc.arg(name), sqlc.arg(status), sqlc.arg(form_type),
    sqlc.arg(heading), sqlc.arg(body), sqlc.arg(button_label), sqlc.arg(success_message),
    sqlc.arg(collect_name), sqlc.arg(consent_kind), sqlc.narg(target_segment_id), sqlc.arg(trigger_flow), sqlc.arg(display_config)
)
RETURNING *;

-- name: UpdateSignupForm :one
UPDATE signup_forms SET
    name = sqlc.arg(name), status = sqlc.arg(status), form_type = sqlc.arg(form_type),
    heading = sqlc.arg(heading), body = sqlc.arg(body), button_label = sqlc.arg(button_label),
    success_message = sqlc.arg(success_message), collect_name = sqlc.arg(collect_name),
    consent_kind = sqlc.arg(consent_kind), target_segment_id = sqlc.narg(target_segment_id),
    trigger_flow = sqlc.arg(trigger_flow), display_config = sqlc.arg(display_config), updated_at = NOW()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: GetSignupForm :one
SELECT * FROM signup_forms WHERE id = sqlc.arg(id);

-- name: DeleteSignupForm :exec
DELETE FROM signup_forms WHERE id = sqlc.arg(id);

-- name: ListSignupForms :many
-- All forms for a channel (dashboard), newest first
SELECT * FROM signup_forms WHERE channel_id = sqlc.arg(channel_id) ORDER BY created_at DESC;

-- name: ListActiveSignupForms :many
-- Active forms for the storefront widget (public)
SELECT * FROM signup_forms WHERE channel_id = sqlc.arg(channel_id) AND status = 'active' ORDER BY created_at;

-- name: RecordSignupSubmission :exec
-- Idempotent per (form, email)
INSERT INTO signup_form_submissions (form_id, channel_id, email, customer_id)
VALUES (sqlc.arg(form_id), sqlc.arg(channel_id), sqlc.arg(email), sqlc.narg(customer_id))
ON CONFLICT (form_id, email) DO UPDATE
    SET customer_id = COALESCE(excluded.customer_id, signup_form_submissions.customer_id);

-- name: CountSignupSubmissions :one
SELECT COUNT(*)::bigint FROM signup_form_submissions WHERE form_id = sqlc.arg(form_id);

-- name: ListSignupSubmissions :many
SELECT id, email, customer_id, created_at FROM signup_form_submissions
WHERE form_id = sqlc.arg(form_id) ORDER BY created_at DESC LIMIT sqlc.arg(lim);

-- name: CreateSignupConfirmation :exec
INSERT INTO signup_confirmations (token_hash, form_id, channel_id, email, first_name, consent_kind, expires_at)
VALUES (sqlc.arg(token_hash), sqlc.arg(form_id), sqlc.arg(channel_id), sqlc.arg(email), sqlc.arg(first_name), sqlc.arg(consent_kind), sqlc.arg(expires_at));

-- name: GetPendingSignupConfirmation :one
SELECT * FROM signup_confirmations
WHERE token_hash = sqlc.arg(token_hash) AND confirmed_at IS NULL AND expires_at > NOW();

-- name: ClaimSignupConfirmation :one
-- Single-use: the first caller flips confirmed_at, every later one matches nothing
UPDATE signup_confirmations SET confirmed_at = NOW()
WHERE token_hash = sqlc.arg(token_hash) AND confirmed_at IS NULL AND expires_at > NOW()
RETURNING *;

-- name: ReleaseSignupConfirmation :exec
UPDATE signup_confirmations SET confirmed_at = NULL WHERE id = sqlc.arg(id);

-- name: SetSignupConfirmationCustomer :exec
UPDATE signup_confirmations SET customer_id = sqlc.arg(customer_id) WHERE id = sqlc.arg(id);

-- name: PruneSignupConfirmations :execrows
-- Unconfirmed links past expiry only - a confirmed row is the record of the double opt-in
DELETE FROM signup_confirmations WHERE confirmed_at IS NULL AND expires_at < sqlc.arg(before);

-- name: DeleteCustomerSignupConfirmations :exec
DELETE FROM signup_confirmations sc
WHERE sc.customer_id = sqlc.arg(customer_id)::uuid
   OR (sc.email = sqlc.arg(email)::text AND sc.channel_id = (SELECT c.channel_id FROM customers c WHERE c.id = sqlc.arg(customer_id)::uuid));
