-- name: EnqueueEmbeddings :exec
INSERT INTO embedding_queue (source, source_id)
SELECT sqlc.arg(source)::text, unnest(sqlc.arg(ids)::uuid[])
ON CONFLICT (source, source_id) DO UPDATE SET queued_at = now(), attempts = 0;

-- name: PendingEmbeddings :many
SELECT source, source_id, queued_at FROM embedding_queue WHERE queued_at <= now() ORDER BY queued_at LIMIT sqlc.arg(lim);

-- An edit since resets it through EnqueueEmbeddings, so only the row as it was read is pushed back
-- name: DeferEmbedding :exec
UPDATE embedding_queue
SET attempts = attempts + 1, queued_at = now() + make_interval(mins => LEAST(1440, power(2, LEAST(attempts, 11))::int))
WHERE source = sqlc.arg(source) AND source_id = sqlc.arg(source_id) AND queued_at = sqlc.arg(queued_at);

-- Only the row as it was read: one queued again while the pass ran stays for the next
-- name: FinishEmbedding :exec
DELETE FROM embedding_queue WHERE source = sqlc.arg(source) AND source_id = sqlc.arg(source_id) AND queued_at = sqlc.arg(queued_at);

-- Products a shopper can find and that have words to embed: listed somewhere, not a variant sold through its parent
-- name: ProductsMissingEmbeddings :many
SELECT p.id FROM catalog_products p
WHERE EXISTS (SELECT 1 FROM catalog_product_channel_listings l WHERE l.product_id = p.id AND l.is_published)
  AND NOT EXISTS (SELECT 1 FROM catalog_product_children c WHERE c.child_id = p.id)
  AND EXISTS (SELECT 1 FROM catalog_product_content pc WHERE pc.product_id = p.id AND pc.name <> '')
  AND NOT EXISTS (SELECT 1 FROM embeddings e WHERE e.source = 'product' AND e.source_id = p.id AND e.model = sqlc.arg(model))
  AND NOT EXISTS (SELECT 1 FROM embedding_queue q WHERE q.source = 'product' AND q.source_id = p.id)
LIMIT sqlc.arg(lim);

-- name: PagesMissingEmbeddings :many
SELECT p.id FROM cms_pages p
WHERE p.is_published AND (p.published_at IS NULL OR p.published_at <= now())
  AND EXISTS (SELECT 1 FROM cms_page_content c WHERE c.page_id = p.id AND c.title <> '')
  AND NOT EXISTS (SELECT 1 FROM embeddings e WHERE e.source = 'cms_page' AND e.source_id = p.id AND e.model = sqlc.arg(model))
  AND NOT EXISTS (SELECT 1 FROM embedding_queue q WHERE q.source = 'cms_page' AND q.source_id = p.id)
LIMIT sqlc.arg(lim);

-- A variant or an unlisted product reads as nothing to embed, so it drops out
-- name: EmbeddingProductContent :many
SELECT pc.product_id, pc.locale, pc.name, pc.short_description, pc.description, pt.name AS product_type, p.attributes
FROM catalog_product_content pc
JOIN catalog_products p ON p.id = pc.product_id
JOIN catalog_product_types pt ON pt.id = p.product_type_id
WHERE pc.product_id = ANY(sqlc.arg(ids)::uuid[])
  AND EXISTS (SELECT 1 FROM catalog_product_channel_listings l WHERE l.product_id = p.id AND l.is_published)
  AND NOT EXISTS (SELECT 1 FROM catalog_product_children c WHERE c.child_id = p.id)
ORDER BY pc.product_id, pc.locale;

-- name: EmbeddingProductCategories :many
SELECT pcat.product_id, cc.locale, cc.name
FROM catalog_product_categories pcat
JOIN catalog_category_content cc ON cc.category_id = pcat.category_id
WHERE pcat.product_id = ANY(sqlc.arg(ids)::uuid[]) AND cc.name <> ''
ORDER BY pcat.product_id, pcat.position, cc.name;

-- Only attributes the storefront shows - a private one must not reach a provider or, later, a shopper's answer
-- name: EmbeddingPublicAttributes :many
SELECT a.id, a.slug, a.name, a.unit, COALESCE(ac.locale, '')::text AS locale, COALESCE(ac.name, '')::text AS local_name
FROM catalog_attributes a
LEFT JOIN catalog_attribute_content ac ON ac.attribute_id = a.id AND ac.name <> ''
WHERE a.is_public AND a.embedded
ORDER BY a.sort_order, a.slug;

-- name: EmbeddingProductAttributeValues :many
SELECT pav.product_id, pav.attribute_id, av.name, COALESCE(avc.locale, '')::text AS locale, COALESCE(avc.name, '')::text AS local_name
FROM catalog_product_attribute_values pav
JOIN catalog_attributes a ON a.id = pav.attribute_id AND a.is_public AND a.embedded
JOIN catalog_attribute_values av ON av.id = pav.attribute_value_id
LEFT JOIN catalog_attribute_value_content avc ON avc.value_id = av.id AND avc.name <> ''
WHERE pav.product_id = ANY(sqlc.arg(ids)::uuid[])
-- The slug breaks a sort_order tie, so one attribute's values arrive together and read as one line
ORDER BY pav.product_id, a.sort_order, a.slug, pav.sort_order, av.sort_order;

-- name: EmbeddingProductLocaleValues :many
SELECT lv.product_id, lv.attribute_id, lv.locale, lv.value
FROM catalog_product_attribute_locale_values lv
JOIN catalog_attributes a ON a.id = lv.attribute_id AND a.is_public AND a.embedded
WHERE lv.product_id = ANY(sqlc.arg(ids)::uuid[]) AND lv.value <> '';

-- An unpublished page reads as nothing to embed, so its passages go
-- name: EmbeddingPageContent :many
SELECT c.page_id, c.locale, c.title, c.meta_description, c.body
FROM cms_page_content c
JOIN cms_pages p ON p.id = c.page_id
WHERE c.page_id = ANY(sqlc.arg(ids)::uuid[]) AND p.is_published AND (p.published_at IS NULL OR p.published_at <= now())
ORDER BY c.page_id, c.locale;

-- name: EmbeddingHashes :many
SELECT source_id, locale, chunk, content_hash FROM embeddings
WHERE source = sqlc.arg(source) AND source_id = ANY(sqlc.arg(ids)::uuid[]) AND model = sqlc.arg(model);

-- name: UpsertEmbedding :exec
INSERT INTO embeddings (source, source_id, locale, model, chunk, content, content_hash, embedding)
VALUES (sqlc.arg(source), sqlc.arg(source_id), sqlc.arg(locale), sqlc.arg(model), sqlc.arg(chunk), sqlc.arg(content), sqlc.arg(content_hash),
    CAST(sqlc.arg(embedding)::text AS vector))
ON CONFLICT (source, source_id, locale, model, chunk)
DO UPDATE SET content = EXCLUDED.content, content_hash = EXCLUDED.content_hash, embedding = EXCLUDED.embedding, updated_at = now();

-- Passages past the document's new end, and locales it no longer has
-- name: TrimEmbeddings :exec
DELETE FROM embeddings
WHERE source = sqlc.arg(source) AND source_id = sqlc.arg(source_id) AND model = sqlc.arg(model)
  AND (NOT (locale = ANY(sqlc.arg(locales)::text[])) OR chunk >= COALESCE((sqlc.arg(chunks)::smallint[])[array_position(sqlc.arg(locales)::text[], locale)], 0));

-- name: DeleteSourceEmbeddings :exec
DELETE FROM embeddings WHERE source = sqlc.arg(source) AND source_id = sqlc.arg(source_id);

-- name: PruneStaleModelEmbeddings :execrows
DELETE FROM embeddings e
USING (SELECT old.source, old.source_id, old.locale, old.model, old.chunk FROM embeddings old WHERE old.model <> sqlc.arg(model) LIMIT sqlc.arg(lim)) stale
WHERE e.source = stale.source AND e.source_id = stale.source_id AND e.locale = stale.locale AND e.model = stale.model AND e.chunk = stale.chunk;

-- name: EmbeddingBacklog :one
SELECT count(*)::bigint AS queued, COALESCE(min(queued_at), now())::timestamptz AS oldest FROM embedding_queue;

-- The store filter runs after the index, so the index has to keep scanning until enough of this store's products turn
-- up - pgvector 0.8's iterative scan, for this transaction only
-- name: AllowIterativeVectorScan :exec
SET LOCAL hnsw.iterative_scan = 'relaxed_order';

-- Nearest first, over the HNSW index. Relaxed order can hand rows back slightly out of order, so they're sorted again
-- name: NearestProducts :many
WITH nearest AS MATERIALIZED (
    SELECT e.source_id, e.locale, e.embedding <=> CAST(sqlc.arg(query)::text AS vector) AS distance
    FROM embeddings e
    WHERE e.source = 'product' AND e.model = sqlc.arg(model) AND e.locale = ANY(sqlc.arg(locales)::text[])
      AND EXISTS (SELECT 1 FROM catalog_product_channel_listings l WHERE l.product_id = e.source_id AND l.channel_id = sqlc.arg(channel_id) AND l.is_published)
    ORDER BY distance
    LIMIT sqlc.arg(lim)
)
SELECT source_id, locale, distance::float8 AS distance FROM nearest ORDER BY distance;

-- A store has a few hundred passages at most, so they're ranked exactly - the + 0 keeps the planner off the index, whose
-- filtered scan would stop early among a hundred thousand product rows
-- name: NearestPassages :many
SELECT e.source_id, e.locale, e.content, p.url_path, c.title, (e.embedding <=> CAST(sqlc.arg(query)::text AS vector))::float8 AS distance
FROM embeddings e
JOIN cms_pages p ON p.id = e.source_id AND p.is_published AND (p.published_at IS NULL OR p.published_at <= now())
JOIN cms_page_channel_listings l ON l.page_id = e.source_id AND l.channel_id = sqlc.arg(channel_id) AND l.is_published
JOIN cms_page_content c ON c.page_id = e.source_id AND c.locale = e.locale
WHERE e.source = 'cms_page' AND e.model = sqlc.arg(model) AND e.locale = ANY(sqlc.arg(locales)::text[])
ORDER BY (e.embedding <=> CAST(sqlc.arg(query)::text AS vector)) + 0
LIMIT sqlc.arg(lim);

-- How much of what one store shows is embedded with the model
-- name: EmbeddingCoverage :one
SELECT
    (SELECT count(*) FROM catalog_products p
     WHERE EXISTS (SELECT 1 FROM catalog_product_channel_listings l WHERE l.product_id = p.id AND l.channel_id = sqlc.arg(channel_id) AND l.is_published)
       AND NOT EXISTS (SELECT 1 FROM catalog_product_children c WHERE c.child_id = p.id)
       AND EXISTS (SELECT 1 FROM catalog_product_content pc WHERE pc.product_id = p.id AND pc.name <> ''))::bigint AS products,
    (SELECT count(*) FROM catalog_products p
     WHERE EXISTS (SELECT 1 FROM catalog_product_channel_listings l WHERE l.product_id = p.id AND l.channel_id = sqlc.arg(channel_id) AND l.is_published)
       AND NOT EXISTS (SELECT 1 FROM catalog_product_children c WHERE c.child_id = p.id)
       AND EXISTS (SELECT 1 FROM embeddings e WHERE e.source = 'product' AND e.source_id = p.id AND e.model = sqlc.arg(model)))::bigint AS embedded_products,
    (SELECT count(*) FROM cms_pages p
     JOIN cms_page_channel_listings l ON l.page_id = p.id AND l.channel_id = sqlc.arg(channel_id) AND l.is_published
     WHERE p.is_published AND (p.published_at IS NULL OR p.published_at <= now()))::bigint AS pages,
    (SELECT count(*) FROM cms_pages p
     JOIN cms_page_channel_listings l ON l.page_id = p.id AND l.channel_id = sqlc.arg(channel_id) AND l.is_published
     WHERE p.is_published AND (p.published_at IS NULL OR p.published_at <= now())
       AND EXISTS (SELECT 1 FROM embeddings e WHERE e.source = 'cms_page' AND e.source_id = p.id AND e.model = sqlc.arg(model)))::bigint AS embedded_pages;

-- The job runs in whichever process claims it, so its last run is read from the schedule, not remembered in memory
-- name: EmbeddingJobLastRun :one
SELECT last_status, last_error, last_run_at FROM scheduled_jobs WHERE name = 'embeddings';

-- A category's or a product type's name is in its products' text, so renaming one queues them
-- name: EnqueueCategoryEmbeddings :exec
INSERT INTO embedding_queue (source, source_id)
SELECT 'product', pc.product_id FROM catalog_product_categories pc WHERE pc.category_id = sqlc.arg(category_id)
ON CONFLICT (source, source_id) DO UPDATE SET queued_at = now(), attempts = 0;

-- name: EnqueueProductTypeEmbeddings :exec
INSERT INTO embedding_queue (source, source_id)
SELECT 'product', p.id FROM catalog_products p WHERE p.product_type_id = sqlc.arg(product_type_id)
  AND NOT EXISTS (SELECT 1 FROM catalog_product_children c WHERE c.child_id = p.id)
ON CONFLICT (source, source_id) DO UPDATE SET queued_at = now(), attempts = 0;

-- What was embedded for each product, in the first of the languages that has it - its public words, for the search overview to read
-- name: EmbeddedProductTexts :many
SELECT DISTINCT ON (source_id) source_id, content FROM embeddings
WHERE source = 'product' AND model = sqlc.arg(model) AND chunk = 0
  AND source_id = ANY(sqlc.arg(ids)::uuid[]) AND locale = ANY(sqlc.arg(locales)::text[])
ORDER BY source_id, array_position(sqlc.arg(locales)::text[], locale);

-- A variant stands for the product it's sold through, the one that's embedded
-- name: ProductParents :many
SELECT child_id, parent_id FROM catalog_product_children WHERE child_id = ANY(sqlc.arg(ids)::uuid[]);

-- The products' vectors averaged, each read in the first of the languages it has - empty when none is embedded
-- name: AverageProductVector :one
SELECT COALESCE(AVG(s.embedding)::text, '')::text AS vector
FROM (
    SELECT DISTINCT ON (e.source_id) e.embedding FROM embeddings e
    WHERE e.source = 'product' AND e.model = sqlc.arg(model) AND e.chunk = 0
      AND e.source_id = ANY(sqlc.arg(ids)::uuid[]) AND e.locale = ANY(sqlc.arg(locales)::text[])
    ORDER BY e.source_id, array_position(sqlc.arg(locales)::text[], e.locale)
) s;

-- Each product's values on the attributes marked to match, its variants' included - a chosen value by its id, a typed one
-- by its lower-cased text, and whether the value stands for all the others
-- name: SimilarMatchValues :many
WITH members AS (
    SELECT p.id AS product_id, p.id AS member_id FROM catalog_products p WHERE p.id = ANY(sqlc.arg(ids)::uuid[])
    UNION ALL
    SELECT c.parent_id, c.child_id FROM catalog_product_children c WHERE c.parent_id = ANY(sqlc.arg(ids)::uuid[])
)
SELECT m.product_id, a.id AS attribute_id, a.name AS attribute_name, a.is_public, ('v:' || pav.attribute_value_id::text)::text AS value, av.matches_any
FROM members m
JOIN catalog_product_attribute_values pav ON pav.product_id = m.member_id
JOIN catalog_attributes a ON a.id = pav.attribute_id AND a.similar_match
JOIN catalog_attribute_values av ON av.id = pav.attribute_value_id
UNION
SELECT m.product_id, a.id, a.name, a.is_public, ('s:' || lower(btrim(p.attributes ->> a.slug)))::text, FALSE
FROM members m
JOIN catalog_products p ON p.id = m.member_id
JOIN catalog_attributes a ON a.similar_match AND jsonb_typeof(p.attributes -> a.slug) IN ('string', 'number', 'boolean')
WHERE btrim(p.attributes ->> a.slug) <> '';
