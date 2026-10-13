-- name: UpsertCustomerConsent :one
-- The history row is written by the same statement, so a change can never land without its audit entry
WITH up AS (
    INSERT INTO customer_consents (customer_id, kind, opted_in, source)
    VALUES (sqlc.arg(customer_id), sqlc.arg(kind), sqlc.arg(opted_in), sqlc.arg(source))
    ON CONFLICT (customer_id, kind) DO UPDATE
      SET opted_in  = EXCLUDED.opted_in,
          source    = EXCLUDED.source,
          updated_at = NOW()
    RETURNING customer_id, kind, opted_in, source, updated_at
), ev AS (
    INSERT INTO customer_consent_events (customer_id, kind, opted_in, source, admin_id)
    SELECT up.customer_id, up.kind, up.opted_in, up.source, sqlc.narg(admin_id)::uuid FROM up
)
SELECT customer_id, kind, opted_in, source, updated_at FROM up;

-- name: GetCustomerConsent :one
SELECT * FROM customer_consents
WHERE customer_id = $1 AND kind = $2;

-- name: ListCustomerConsents :many
SELECT * FROM customer_consents
WHERE customer_id = $1;

-- name: ListCustomerConsentEvents :many
SELECT * FROM customer_consent_events
WHERE customer_id = sqlc.arg(customer_id)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(lim);

-- name: DeleteCustomerConsents :exec
-- The GDPR erasure path - FK CASCADE only fires on a hard delete, and erasure anonymizes the row instead
DELETE FROM customer_consents WHERE customer_id = $1;

-- name: DeleteCustomerConsentEvents :exec
DELETE FROM customer_consent_events WHERE customer_id = $1;
