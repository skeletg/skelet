-- Admin audit trail

-- name: InsertAuditLog :exec
INSERT INTO audit_log (admin_id, admin_email, action, detail, ip, success, duration_ms)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: ListAuditLog :many
SELECT * FROM audit_log
WHERE (sqlc.narg(admin_id)::uuid IS NULL OR admin_id = sqlc.narg(admin_id)::uuid)
  AND (sqlc.narg(action)::text IS NULL OR action = sqlc.narg(action)::text)
ORDER BY created_at DESC
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: CountAuditLog :one
SELECT COUNT(*) FROM audit_log
WHERE (sqlc.narg(admin_id)::uuid IS NULL OR admin_id = sqlc.narg(admin_id)::uuid)
  AND (sqlc.narg(action)::text IS NULL OR action = sqlc.narg(action)::text);

-- name: PruneAuditLog :execrows
DELETE FROM audit_log WHERE created_at < $1;
