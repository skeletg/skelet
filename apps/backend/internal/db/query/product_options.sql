-- name: CreateProductOption :one
INSERT INTO catalog_product_options (
    product_id, input_type, is_required, sort_order,
    price_value, price_type, max_chars, max_file_bytes, file_extensions,
    visible_if
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING *;

-- name: GetProductOptionByID :one
SELECT * FROM catalog_product_options WHERE id = $1;

-- name: ListProductOptionsByProductID :many
SELECT * FROM catalog_product_options
WHERE product_id = $1
ORDER BY sort_order, created_at;

-- name: UpdateProductOption :one
UPDATE catalog_product_options
SET input_type      = $2,
    is_required     = $3,
    sort_order      = $4,
    price_value     = $5,
    price_type      = $6,
    max_chars       = $7,
    max_file_bytes  = $8,
    file_extensions = $9,
    visible_if      = $10,
    updated_at      = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteProductOption :exec
DELETE FROM catalog_product_options WHERE id = $1;

-- name: UpsertProductOptionContent :one
INSERT INTO catalog_product_option_content (option_id, locale, title, help_text)
VALUES ($1, $2, $3, $4)
ON CONFLICT (option_id, locale)
DO UPDATE SET title = EXCLUDED.title,
              help_text = EXCLUDED.help_text
RETURNING *;

-- name: ListProductOptionContent :many
SELECT * FROM catalog_product_option_content
WHERE option_id = $1
ORDER BY locale;

-- name: ListProductOptionContentByOptionIDs :many
SELECT * FROM catalog_product_option_content
WHERE option_id = ANY(sqlc.arg(option_ids)::uuid[])
ORDER BY option_id, locale;

-- name: DeleteProductOptionContent :exec
DELETE FROM catalog_product_option_content
WHERE option_id = $1 AND locale = $2;

-- name: CreateProductOptionValue :one
INSERT INTO catalog_product_option_values (
    option_id, sort_order, price_value, price_type, is_default, source_product_id, quantity, forced_line_qty
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetProductOptionValueByID :one
SELECT * FROM catalog_product_option_values WHERE id = $1;

-- name: ListProductOptionValuesByOptionID :many
SELECT * FROM catalog_product_option_values
WHERE option_id = $1
ORDER BY sort_order, id;

-- name: ListProductOptionValuesByOptionIDs :many
SELECT * FROM catalog_product_option_values
WHERE option_id = ANY(sqlc.arg(option_ids)::uuid[])
ORDER BY option_id, sort_order, id;

-- name: CountProductOptionValueDefaultsForOption :one
SELECT COUNT(*) FROM catalog_product_option_values
WHERE option_id = $1 AND is_default = TRUE;

-- name: UpdateProductOptionValue :one
UPDATE catalog_product_option_values
SET sort_order        = $2,
    price_value       = $3,
    price_type        = $4,
    is_default        = $5,
    source_product_id = $6,
    quantity          = $7,
    forced_line_qty   = $8,
    updated_at        = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteProductOptionValue :exec
DELETE FROM catalog_product_option_values WHERE id = $1;

-- name: UpsertProductOptionValueContent :one
INSERT INTO catalog_product_option_value_content (value_id, locale, title)
VALUES ($1, $2, $3)
ON CONFLICT (value_id, locale)
DO UPDATE SET title = EXCLUDED.title
RETURNING *;

-- name: ListProductOptionValueContent :many
SELECT * FROM catalog_product_option_value_content
WHERE value_id = $1
ORDER BY locale;

-- name: ListProductOptionValueContentByValueIDs :many
SELECT * FROM catalog_product_option_value_content
WHERE value_id = ANY(sqlc.arg(value_ids)::uuid[])
ORDER BY value_id, locale;

-- name: DeleteProductOptionValueContent :exec
DELETE FROM catalog_product_option_value_content
WHERE value_id = $1 AND locale = $2;
