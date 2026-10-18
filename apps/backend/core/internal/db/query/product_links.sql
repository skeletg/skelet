-- Curated product links + also-bought co-occurrence

-- name: ListProductLinks :many
SELECT * FROM catalog_product_links
WHERE product_id = $1 AND kind = $2
ORDER BY sort_order, created_at;

-- name: DeleteProductLinks :exec
DELETE FROM catalog_product_links WHERE product_id = $1 AND kind = $2;

-- name: InsertProductLink :exec
INSERT INTO catalog_product_links (product_id, linked_product_id, kind, sort_order)
VALUES ($1, $2, $3, $4)
ON CONFLICT DO NOTHING;

-- name: ListProductLinksForProducts :many
-- The cart rail's crosssell candidates for every cart product in one read, each product's links in curated order
SELECT product_id, linked_product_id FROM catalog_product_links
WHERE  product_id = ANY(sqlc.arg(product_ids)::uuid[]) AND kind = sqlc.arg(kind)
ORDER BY product_id, sort_order, created_at;

-- name: ClearAlsoBought :exec
-- Its own statement, run in the same transaction as RebuildAlsoBought - a DELETE inside the rebuild's CTE ran but was INVISIBLE to the INSERT (data-modifying CTEs share one snapshot), so every rebuild after the first died on the primary key
DELETE FROM product_also_bought;

-- name: RebuildAlsoBought :exec
-- Recomputes the co-occurrence table from completed (non-cancelled) order lines: variants roll up to their configurable parent, pairs score by the number of distinct orders since `since` that bought both, and each product keeps its top_n - full rebuild, the job runs off-peak
WITH bought AS (
    SELECT DISTINCT ol.order_id, COALESCE(pc.parent_id, p.id) AS pid
    FROM order_lines ol
    JOIN catalog_products p ON p.sku = ol.product_sku
    JOIN orders o ON o.id = ol.order_id
    LEFT JOIN catalog_product_children pc ON pc.child_id = p.id
    WHERE o.status NOT IN ('cancelled', 'draft') AND ol.removed_at IS NULL
      AND o.created_at >= sqlc.arg(since)::timestamptz
),
pairs AS (
    SELECT a.pid AS product_id, b.pid AS other_product_id, COUNT(DISTINCT a.order_id)::int AS score
    FROM bought a
    JOIN bought b ON a.order_id = b.order_id AND a.pid <> b.pid
    GROUP BY a.pid, b.pid
),
ranked AS (
    SELECT *, ROW_NUMBER() OVER (PARTITION BY product_id ORDER BY score DESC, other_product_id) AS rn
    FROM pairs
)
INSERT INTO product_also_bought (product_id, other_product_id, score)
SELECT product_id, other_product_id, score FROM ranked WHERE rn <= sqlc.arg(top_n)::int;

-- name: ListAlsoBought :many
SELECT other_product_id FROM product_also_bought
WHERE product_id = $1
ORDER BY score DESC, other_product_id
LIMIT $2;

-- name: ListAlsoBoughtForProducts :many
SELECT product_id, other_product_id FROM (
    SELECT product_id, other_product_id,
           ROW_NUMBER() OVER (PARTITION BY product_id ORDER BY score DESC, other_product_id) AS rn
    FROM   product_also_bought
    WHERE  product_id = ANY(sqlc.arg(product_ids)::uuid[])
) ranked
WHERE  rn <= sqlc.arg(per_product)::int
ORDER BY product_id, rn;
