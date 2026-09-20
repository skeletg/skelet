-- name: UpsertCustomerConsent :one
INSERT INTO customer_consents (customer_id, kind, opted_in, source)
VALUES ($1, $2, $3, $4)
ON CONFLICT (customer_id, kind) DO UPDATE
  SET opted_in  = EXCLUDED.opted_in,
      source    = EXCLUDED.source,
      updated_at = NOW()
RETURNING *;

-- name: GetCustomerConsent :one
SELECT * FROM customer_consents
WHERE customer_id = $1 AND kind = $2;

-- name: ListCustomerConsents :many
SELECT * FROM customer_consents
WHERE customer_id = $1;

-- name: ListCustomerConsentsByCustomerIDs :many
-- Batch of GetCustomerConsent for ONE kind across many customers - mass-send consent gating resolves the whole recipient page in one query instead of a per-recipient lookup - customers with no row are simply absent from the result; the caller applies the definition default (GDPR-safe opt-out)
SELECT * FROM customer_consents
WHERE kind = sqlc.arg(kind) AND customer_id = ANY(sqlc.arg(customer_ids)::uuid[]);

-- name: DeleteCustomerConsents :exec
-- Used by future GDPR delete flow - FK CASCADE also handles this, but an
-- explicit query lets the service log which consents were wiped
DELETE FROM customer_consents WHERE customer_id = $1;
