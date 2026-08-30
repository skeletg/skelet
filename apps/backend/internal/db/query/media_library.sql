-- name: CreateMediaAsset :one
INSERT INTO media_assets (
    id, directory_prefix, filename, object_key,
    mime_type, size_bytes, width, height, status,
    title, alt_text, caption, notes, created_by, updated_by
) VALUES (
    $1, $2, $3, $4,
    $5, $6, $7, $8, $9,
    $10, $11, $12, $13, $14, $15
)
RETURNING *;

-- name: GetMediaAssetByID :one
SELECT * FROM media_assets WHERE id = $1;

-- name: GetMediaAssetsByIDs :many
SELECT * FROM media_assets WHERE id = ANY(sqlc.arg(ids)::uuid[]);

-- name: ListMediaAssetsByDirectory :many
SELECT * FROM media_assets
WHERE
    ($1::text = '' AND object_key NOT LIKE '%/%') OR
    ($1::text != '' AND object_key LIKE $1::text || '%' AND object_key NOT LIKE $1::text || '%/%')
ORDER BY created_at DESC, filename;

-- name: ListMediaAssetsByPathRecursive :many
SELECT * FROM media_assets
WHERE $1 = '' OR object_key LIKE $1 || '%'
ORDER BY object_key, filename;

-- name: SearchMediaAssets :many
SELECT * FROM media_assets
WHERE
    ($1 = '' OR object_key LIKE $1 || '%')
    AND ($2 = '' OR mime_type LIKE $2 || '/%')
    AND (
        $3 = ''
        OR filename ILIKE '%' || $3 || '%'
        OR title ILIKE '%' || $3 || '%'
        OR alt_text ILIKE '%' || $3 || '%'
        OR caption ILIKE '%' || $3 || '%'
        OR notes ILIKE '%' || $3 || '%'
    )
ORDER BY updated_at DESC, filename
LIMIT $4 OFFSET $5;

-- name: CountSearchMediaAssets :one
SELECT count(*) FROM media_assets
WHERE
    ($1 = '' OR object_key LIKE $1 || '%')
    AND ($2 = '' OR mime_type LIKE $2 || '/%')
    AND (
        $3 = ''
        OR filename ILIKE '%' || $3 || '%'
        OR title ILIKE '%' || $3 || '%'
        OR alt_text ILIKE '%' || $3 || '%'
        OR caption ILIKE '%' || $3 || '%'
        OR notes ILIKE '%' || $3 || '%'
    );

-- name: ConfirmMediaAssetUpload :one
UPDATE media_assets
SET
    status = 'ready',
    width = $2,
    height = $3,
    updated_by = $4,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: UpdateMediaAssetMetadata :one
UPDATE media_assets
SET
    title = $2,
    alt_text = $3,
    caption = $4,
    notes = $5,
    updated_by = $6,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: UpdateMediaAssetLocation :one
UPDATE media_assets
SET
    directory_prefix = $2,
    filename = $3,
    object_key = $4,
    updated_by = $5,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteMediaAsset :exec
DELETE FROM media_assets WHERE id = $1;

-- name: DeleteMediaAssetsByDirectoryPrefix :exec
DELETE FROM media_assets
WHERE $1 = '' OR object_key LIKE $1 || '%';

-- name: SetMediaFocalPoint :one
UPDATE media_assets
SET focal_x = $2, focal_y = $3
WHERE id = $1
RETURNING *;

-- name: GetMediaAssetByObjectKey :one
SELECT * FROM media_assets WHERE object_key = $1 AND status = 'active';

-- name: ListOrphanProductOptionAssets :many
-- Customer option uploads that never made it onto a line (or whose line was
-- removed) - cart and order line options snapshot the asset's PUBLIC URL, so
-- reference = any file_url ending in the object key - order rows never delete,
-- which correctly pins assets referenced by placed orders forever
SELECT * FROM media_assets
WHERE media_assets.object_key LIKE 'product-options/%'
  AND media_assets.created_at < sqlc.arg(cutoff)
  AND NOT EXISTS (
    SELECT 1 FROM cart_line_options c
    WHERE c.file_url != '' AND c.file_url LIKE '%' || media_assets.object_key
  )
  AND NOT EXISTS (
    SELECT 1 FROM order_line_options o
    WHERE o.file_url != '' AND o.file_url LIKE '%' || media_assets.object_key
  )
ORDER BY media_assets.created_at
LIMIT sqlc.arg(max_rows);
