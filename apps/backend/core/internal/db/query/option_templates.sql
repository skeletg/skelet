-- name: CreateOptionTemplate :one
INSERT INTO catalog_option_templates (name, description, currency_code)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetOptionTemplate :one
SELECT * FROM catalog_option_templates WHERE id = $1;

-- name: UpdateOptionTemplate :one
UPDATE catalog_option_templates
SET name = $2, description = $3, currency_code = $4, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteOptionTemplate :exec
DELETE FROM catalog_option_templates WHERE id = $1;

-- name: ListOptionTemplates :many
SELECT t.*,
       (SELECT COUNT(*) FROM catalog_product_options o WHERE o.template_id = t.id)::int AS option_count,
       (SELECT COUNT(*) FROM catalog_product_option_rules r WHERE r.template_id = t.id)::int AS rule_count,
       (SELECT COUNT(*) FROM catalog_option_template_products a WHERE a.template_id = t.id)::int AS product_count,
       (SELECT COUNT(*) FROM catalog_option_template_product_types pt WHERE pt.template_id = t.id)::int AS product_type_count
FROM catalog_option_templates t
ORDER BY lower(t.name);

-- name: ListOptionTemplatesByIDs :many
SELECT t.*,
       (SELECT COUNT(*) FROM catalog_product_options o WHERE o.template_id = t.id)::int AS option_count,
       (SELECT COUNT(*) FROM catalog_product_option_rules r WHERE r.template_id = t.id)::int AS rule_count,
       (SELECT COUNT(*) FROM catalog_option_template_products a WHERE a.template_id = t.id)::int AS product_count,
       (SELECT COUNT(*) FROM catalog_option_template_product_types pt WHERE pt.template_id = t.id)::int AS product_type_count
FROM catalog_option_templates t
WHERE t.id = ANY(sqlc.arg(ids)::uuid[]);

-- name: ListOptionTemplateAssignmentsByProductIDs :many
SELECT product_id, template_id FROM catalog_option_template_products
WHERE product_id = ANY(sqlc.arg(product_ids)::uuid[])
ORDER BY product_id, "position", created_at;

-- name: DeleteOptionTemplateAssignmentsForProduct :exec
DELETE FROM catalog_option_template_products WHERE product_id = $1;

-- name: InsertOptionTemplateAssignment :exec
INSERT INTO catalog_option_template_products (product_id, template_id, "position")
VALUES ($1, $2, $3);

-- name: AppendOptionTemplateAssignment :exec
INSERT INTO catalog_option_template_products (product_id, template_id, "position")
VALUES ($1, $2, COALESCE((SELECT MAX("position") + 1 FROM catalog_option_template_products WHERE product_id = $1), 0))
ON CONFLICT (product_id, template_id) DO NOTHING;

-- name: DeleteOptionTemplateAssignment :exec
DELETE FROM catalog_option_template_products WHERE product_id = $1 AND template_id = $2;

-- name: ListProductIDsUsingOptionTemplate :many
SELECT product_id FROM catalog_option_template_products
WHERE template_id = $1
ORDER BY created_at, product_id
LIMIT $2 OFFSET $3;

-- name: ListAllProductIDsUsingOptionTemplate :many
-- Every product it reaches - picked on the product, or through the type of a product of a kind that carries options
SELECT a.product_id FROM catalog_option_template_products a WHERE a.template_id = sqlc.arg(template_id)
UNION
SELECT p.id FROM catalog_products p
JOIN catalog_option_template_product_types pt ON pt.product_type_id = p.product_type_id
WHERE pt.template_id = sqlc.arg(template_id) AND p.kind::text = ANY(sqlc.arg(kinds)::text[]);

-- name: ListEffectiveOptionTemplatesByProductIDs :many
-- A product's templates as its options show them: its type's first, in the type's order, then its own - only kinds
-- that carry options get the type's. A template on both comes back twice, the caller keeps the first
SELECT x.product_id, x.template_id, x.from_type
FROM (
    SELECT p.id AS product_id, pt.template_id, true AS from_type, 0 AS tier, pt."position", pt.created_at
    FROM catalog_products p
    JOIN catalog_option_template_product_types pt ON pt.product_type_id = p.product_type_id
    WHERE p.id = ANY(sqlc.arg(product_ids)::uuid[]) AND p.kind::text = ANY(sqlc.arg(kinds)::text[])
    UNION ALL
    SELECT a.product_id, a.template_id, false, 1, a."position", a.created_at
    FROM catalog_option_template_products a
    WHERE a.product_id = ANY(sqlc.arg(product_ids)::uuid[])
) x
ORDER BY x.product_id, x.tier, x."position", x.created_at;

-- name: ListOptionTemplateIDsByProductTypeIDs :many
SELECT product_type_id, template_id FROM catalog_option_template_product_types
WHERE product_type_id = ANY(sqlc.arg(product_type_ids)::uuid[])
ORDER BY product_type_id, "position", created_at;

-- name: DeleteOptionTemplateAssignmentsForProductType :exec
DELETE FROM catalog_option_template_product_types WHERE product_type_id = $1;

-- name: InsertOptionTemplateProductTypeAssignment :exec
INSERT INTO catalog_option_template_product_types (product_type_id, template_id, "position")
VALUES ($1, $2, $3);

-- name: ListProductTypeIDsUsingOptionTemplate :many
SELECT pt.product_type_id FROM catalog_option_template_product_types pt
JOIN catalog_product_types t ON t.id = pt.product_type_id
WHERE pt.template_id = $1
ORDER BY lower(t.name), t.id;

-- name: CountProductTypesUsingOptionTemplate :one
SELECT COUNT(*) FROM catalog_option_template_product_types WHERE template_id = $1;

-- name: ListOptionCarryingProductIDsByType :many
-- The products a type's templates reach
SELECT id FROM catalog_products
WHERE product_type_id = sqlc.arg(product_type_id) AND kind::text = ANY(sqlc.arg(kinds)::text[])
ORDER BY id;

-- name: CountProductsUsingOptionTemplate :one
SELECT COUNT(*) FROM catalog_option_template_products WHERE template_id = $1;

-- name: ListProductOptionsByTemplateIDs :many
SELECT * FROM catalog_product_options
WHERE template_id = ANY(sqlc.arg(template_ids)::uuid[])
ORDER BY template_id, sort_order, created_at;

-- name: ListProductOptionRulesByTemplateIDs :many
SELECT * FROM catalog_product_option_rules
WHERE template_id = ANY(sqlc.arg(template_ids)::uuid[])
ORDER BY template_id, sort_order, created_at;

-- name: GetProductConfiguratorLayout :one
SELECT configurator_layout FROM catalog_products WHERE id = $1;

-- name: SetProductConfiguratorLayout :execrows
UPDATE catalog_products SET configurator_layout = $2, updated_at = NOW() WHERE id = $1;
