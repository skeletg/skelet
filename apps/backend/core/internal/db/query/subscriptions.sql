-- Catalog: a subscription product's recurring terms (admin-configured, 1:1)

-- name: UpsertSubscriptionTerms :one
INSERT INTO catalog_subscription_terms (product_id, interval_unit, interval_count, trial_days)
VALUES ($1, $2, $3, $4)
ON CONFLICT (product_id) DO UPDATE
SET interval_unit  = EXCLUDED.interval_unit,
    interval_count = EXCLUDED.interval_count,
    trial_days     = EXCLUDED.trial_days,
    updated_at     = NOW()
RETURNING *;

-- name: GetSubscriptionTerms :one
SELECT * FROM catalog_subscription_terms WHERE product_id = $1;

-- name: DeleteSubscriptionTerms :exec
DELETE FROM catalog_subscription_terms WHERE product_id = $1;

-- Subscription instances (one per purchased subscription line)

-- name: CreateSubscription :one
INSERT INTO subscriptions (
    customer_id, product_id, order_id, interval_unit, interval_count,
    price, currency_code, quantity, sku, title,
    current_period_start, current_period_end, next_billing_at, channel_id
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
RETURNING *;

-- name: GetSubscriptionByID :one
SELECT * FROM subscriptions WHERE id = $1;

-- name: GetSubscriptionForUpdate :one
-- Row-locked read used when billing/cancelling so period advancement is
-- serialized against concurrent schedulers and customer actions
SELECT * FROM subscriptions WHERE id = $1 FOR UPDATE;

-- name: ListSubscriptionsByOrder :many
SELECT * FROM subscriptions WHERE order_id = $1 ORDER BY created_at;

-- name: ListSubscriptionsByCustomer :many
SELECT * FROM subscriptions
WHERE customer_id = $1
ORDER BY created_at DESC;

-- name: ListSubscriptions :many
SELECT * FROM subscriptions
WHERE (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: ListSubscriptionsForChannels :many
SELECT * FROM subscriptions
WHERE (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
  AND channel_id = ANY(sqlc.arg(channel_ids)::uuid[])
ORDER BY created_at DESC
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: CountSubscriptions :one
-- The total behind ListSubscriptions / ListSubscriptionsForChannels - a nil channel_ids is every channel
SELECT COUNT(*)::INTEGER FROM subscriptions
WHERE (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
  AND (sqlc.narg(channel_ids)::uuid[] IS NULL OR channel_id = ANY(sqlc.narg(channel_ids)::uuid[]));

-- name: ListDueSubscriptions :many
-- The scheduler's hot path: active subscriptions whose next charge is due
SELECT * FROM subscriptions
WHERE status = 'active' AND next_billing_at <= $1
ORDER BY next_billing_at
LIMIT $2;

-- name: AdvanceSubscriptionPeriod :one
UPDATE subscriptions
SET current_period_start = $2,
    current_period_end   = $3,
    next_billing_at      = $4,
    updated_at           = NOW()
WHERE id = $1
RETURNING *;

-- name: SetSubscriptionStatus :one
UPDATE subscriptions
SET status      = $2,
    canceled_at = $3,
    updated_at  = NOW()
WHERE id = $1
RETURNING *;

-- Subscription invoices (one per billing period; append-only)

-- name: CreateSubscriptionInvoice :many
-- Idempotent: ON CONFLICT makes a period billable only once even if the
-- scheduler re-runs - returns 0 rows when the period was already billed
INSERT INTO subscription_invoices (
    subscription_id, period_start, period_end, amount, currency_code, sequence
)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (subscription_id, period_start) DO NOTHING
RETURNING *;

-- name: CountSubscriptionInvoices :one
SELECT COUNT(*) FROM subscription_invoices WHERE subscription_id = $1;

-- name: ListSubscriptionInvoices :many
SELECT * FROM subscription_invoices
WHERE subscription_id = $1
ORDER BY sequence;

-- name: ListChargeableInvoices :many
-- Due invoices whose next dunning attempt has arrived (NULL = never
-- attempted) - drives renewal charging
SELECT * FROM subscription_invoices
WHERE  status = 'due'
  AND  (next_attempt_at IS NULL OR next_attempt_at <= sqlc.arg(now)::timestamptz)
ORDER  BY period_start
LIMIT  sqlc.arg(batch)::int;

-- name: MarkInvoicePaid :exec
UPDATE subscription_invoices
SET    status = 'paid', paid_at = now(), payment_reference = $2, failure_reason = ''
WHERE  id = $1;

-- name: RecordInvoiceAttempt :one
-- Bumps the dunning counter; next_attempt_at NULL parks the invoice
-- (retries exhausted - caller cancels the subscription)
UPDATE subscription_invoices
SET    attempt_count = attempt_count + 1,
       next_attempt_at = sqlc.narg(next_attempt_at),
       failure_reason = $2
WHERE  id = $1
RETURNING *;

-- name: SetSubscriptionPaymentMethod :one
UPDATE subscriptions
SET    payment_method_id = sqlc.narg(payment_method_id), updated_at = now()
WHERE  id = $1
RETURNING *;
