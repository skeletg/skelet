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

-- name: ClearAlsoBought :exec
-- Its own statement, run in the same transaction as RebuildAlsoBought - a DELETE inside the rebuild's CTE ran but was INVISIBLE to the INSERT (data-modifying CTEs share one snapshot), so every rebuild after the first died on the primary key
DELETE FROM product_also_bought;

-- name: RebuildAlsoBought :exec
-- Recomputes the co-occurrence table from completed (non-cancelled) order lines: pairs of distinct products bought in the same order, scored by pair frequency, top 20 per product - full rebuild - the table is small relative to orders and the job runs off-peak
WITH pairs AS (
    SELECT a.pid AS product_id, b.pid AS other_product_id, COUNT(*)::int AS score
    FROM (
        SELECT ol.order_id, p.id AS pid
        FROM order_lines ol
        JOIN catalog_products p ON p.sku = ol.product_sku
        JOIN orders o ON o.id = ol.order_id
        WHERE o.status NOT IN ('cancelled', 'draft') AND ol.removed_at IS NULL
    ) a
    JOIN (
        SELECT ol.order_id, p.id AS pid
        FROM order_lines ol
        JOIN catalog_products p ON p.sku = ol.product_sku
        JOIN orders o ON o.id = ol.order_id
        WHERE o.status NOT IN ('cancelled', 'draft') AND ol.removed_at IS NULL
    ) b ON a.order_id = b.order_id AND a.pid <> b.pid
    GROUP BY a.pid, b.pid
),
ranked AS (
    SELECT *, ROW_NUMBER() OVER (PARTITION BY product_id ORDER BY score DESC) AS rn
    FROM pairs
)
INSERT INTO product_also_bought (product_id, other_product_id, score)
SELECT product_id, other_product_id, score FROM ranked WHERE rn <= 20;

-- name: ListAlsoBought :many
SELECT other_product_id FROM product_also_bought
WHERE product_id = $1
ORDER BY score DESC
LIMIT $2;
