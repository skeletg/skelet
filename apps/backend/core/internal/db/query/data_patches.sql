-- name: ListAppliedDataPatches :many
SELECT id FROM data_patches ORDER BY id;

-- name: InsertDataPatch :exec
INSERT INTO data_patches (id, namespace) VALUES ($1, $2);
