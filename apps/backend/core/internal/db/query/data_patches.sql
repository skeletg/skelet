-- name: ListAppliedDataPatches :many
SELECT id FROM data_patches ORDER BY id;

-- name: InsertDataPatch :exec
INSERT INTO data_patches (id, namespace) VALUES ($1, $2);

-- name: LockDataPatches :exec
SELECT pg_advisory_xact_lock(sqlc.arg(class)::int, 0);

-- name: DataPatchApplied :one
SELECT EXISTS (SELECT 1 FROM data_patches WHERE id = $1);
