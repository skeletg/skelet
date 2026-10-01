-- name: CreateGroupedMember :one
INSERT INTO catalog_product_grouped_members (
    parent_id, member_id, default_quantity, sort_order
)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetGroupedMemberByID :one
SELECT * FROM catalog_product_grouped_members WHERE id = $1;

-- name: ListGroupedMembersByParentID :many
SELECT * FROM catalog_product_grouped_members
WHERE parent_id = $1
ORDER BY sort_order, created_at;

-- name: UpdateGroupedMember :one
UPDATE catalog_product_grouped_members
SET default_quantity = $2,
    sort_order       = $3,
    updated_at       = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteGroupedMember :exec
DELETE FROM catalog_product_grouped_members WHERE id = $1;

-- name: ListGroupedMemberLinksByMemberIDs :many
-- Inverse: every grouped product a given member belongs to - batched; powers
-- Product.partOf.groupedProducts - A product may be a member of many groups
SELECT member_id, parent_id, default_quantity, sort_order
FROM catalog_product_grouped_members
WHERE member_id = ANY(sqlc.arg(member_ids)::uuid[])
ORDER BY member_id, sort_order, created_at;
