-- Catalog: a product's downloadable files (admin-configured)

-- name: CreateProductDownload :one
INSERT INTO catalog_product_downloads (product_id, title, url, max_downloads, sort_order)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetProductDownloadByID :one
SELECT * FROM catalog_product_downloads WHERE id = $1;

-- name: ListProductDownloadsByProductID :many
SELECT * FROM catalog_product_downloads
WHERE product_id = $1
ORDER BY sort_order, created_at;

-- name: UpdateProductDownload :one
UPDATE catalog_product_downloads
SET title = $2, url = $3, max_downloads = $4, sort_order = $5, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteProductDownload :exec
DELETE FROM catalog_product_downloads WHERE id = $1;

-- Download grants (per-buyer access, created at order placement)

-- name: CreateDownloadGrant :one
INSERT INTO download_grants (
    token, order_id, customer_id, email, product_id, download_id,
    title, url, max_downloads, expires_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING *;

-- name: GetDownloadGrantByToken :one
SELECT * FROM download_grants WHERE token = $1;

-- name: GetDownloadGrantByTokenForUpdate :one
-- Row-locked read used when consuming a download so the use-count check is
-- serialized against concurrent downloads of the same grant
SELECT * FROM download_grants WHERE token = $1 FOR UPDATE;

-- name: IncrementDownloadGrantUse :one
UPDATE download_grants
SET downloads_used = downloads_used + 1
WHERE id = $1
RETURNING *;

-- name: ListDownloadGrantsByOrder :many
SELECT * FROM download_grants WHERE order_id = $1 ORDER BY created_at;

-- name: ListDownloadGrantsByCustomer :many
SELECT * FROM download_grants
WHERE customer_id = $1
ORDER BY created_at DESC;
