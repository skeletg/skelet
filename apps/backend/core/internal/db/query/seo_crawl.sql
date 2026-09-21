-- name: CreateSeoCrawl :one
INSERT INTO seo_crawls (channel_id, base_url, max_pages, max_depth, respect_robots, admin_id)
VALUES (@channel_id, @base_url, @max_pages, @max_depth, @respect_robots, @admin_id)
RETURNING *;

-- name: RunningSeoCrawl :one
SELECT * FROM seo_crawls WHERE channel_id = @channel_id AND status = 'running' ORDER BY started_at DESC LIMIT 1;

-- name: GetSeoCrawl :one
SELECT * FROM seo_crawls WHERE id = @id;

-- name: LatestSeoCrawl :one
SELECT * FROM seo_crawls
WHERE channel_id = @channel_id AND status = 'completed'
ORDER BY started_at DESC LIMIT 1;

-- name: ListSeoCrawls :many
SELECT * FROM seo_crawls WHERE channel_id = @channel_id ORDER BY started_at DESC LIMIT @row_limit;

-- name: UpdateSeoCrawlProgress :exec
UPDATE seo_crawls SET pages_crawled = @pages_crawled, links_found = @links_found, progress_at = now() WHERE id = @id;

-- name: FinishSeoCrawl :exec
UPDATE seo_crawls
SET status = @status, error = @error, hit_budget = @hit_budget,
    pages_crawled = @pages_crawled, links_found = @links_found, finished_at = now()
WHERE id = @id;

-- name: CancelStaleSeoCrawls :exec
UPDATE seo_crawls SET status = 'cancelled', finished_at = now()
WHERE status = 'running' AND progress_at < now() - make_interval(mins => sqlc.arg(silent_for_minutes)::int);

-- name: InsertSeoCrawlPages :exec
INSERT INTO seo_crawl_pages (crawl_id, path, lookup_path, status_code, title, meta_description, robots_meta,
    headings, word_count, kind, internal_links, external_links, noindex, nofollow, canonical, redirect_to,
    depth, fetch_ms, byte_size)
SELECT sqlc.arg(crawl_id)::uuid,
       unnest(sqlc.arg(paths)::text[]),
       unnest(sqlc.arg(lookup_paths)::text[]),
       unnest(sqlc.arg(status_codes)::int[]),
       unnest(sqlc.arg(titles)::text[]),
       unnest(sqlc.arg(meta_descriptions)::text[]),
       unnest(sqlc.arg(robots_metas)::text[]),
       unnest(sqlc.arg(headings)::text[])::jsonb,
       unnest(sqlc.arg(word_counts)::int[]),
       unnest(sqlc.arg(kinds)::text[]),
       unnest(sqlc.arg(internal_links)::int[]),
       unnest(sqlc.arg(external_links)::int[]),
       unnest(sqlc.arg(noindexes)::boolean[]),
       unnest(sqlc.arg(nofollows)::boolean[]),
       unnest(sqlc.arg(canonicals)::text[]),
       unnest(sqlc.arg(redirects)::text[]),
       unnest(sqlc.arg(depths)::int[]),
       unnest(sqlc.arg(fetch_times)::int[]),
       unnest(sqlc.arg(byte_sizes)::int[])
ON CONFLICT (crawl_id, path) DO NOTHING;

-- name: InsertSeoCrawlLinks :exec
INSERT INTO seo_crawl_links (crawl_id, from_path, to_path, occurrences, nofollow, anchor)
SELECT sqlc.arg(crawl_id)::uuid,
       unnest(sqlc.arg(from_paths)::text[]),
       unnest(sqlc.arg(to_paths)::text[]),
       unnest(sqlc.arg(occurrence_counts)::int[]),
       unnest(sqlc.arg(nofollows)::boolean[]),
       unnest(sqlc.arg(anchors)::text[])
ON CONFLICT (crawl_id, from_path, to_path)
DO UPDATE SET occurrences = seo_crawl_links.occurrences + excluded.occurrences;

-- Entity resolution runs after the fetch so the crawler stays a pure HTTP job - the join is on the same
-- channel-override rules the storefront router uses
-- name: ResolveSeoCrawlEntities :exec
UPDATE seo_crawl_pages p
SET entity_type = c.entity_type, entity_id = c.entity_id
FROM (
    SELECT DISTINCT ON (request_path) request_path, entity_type, entity_id
    FROM url_rewrites
    WHERE redirect_type = 0 AND (channel_id = @channel_id::uuid OR channel_id IS NULL)
    ORDER BY request_path, (channel_id IS NOT NULL) DESC
) c
WHERE p.crawl_id = @crawl_id AND c.request_path = p.lookup_path;

-- name: SeoCrawlPages :many
SELECT p.path, p.lookup_path, p.status_code, p.title, p.entity_type, p.entity_id, p.noindex, p.canonical,
       p.redirect_to, p.depth, p.kind,
       (SELECT count(*) FROM seo_crawl_links l WHERE l.crawl_id = p.crawl_id AND l.to_path = p.path)::int AS in_links,
       (SELECT count(*) FROM seo_crawl_links l WHERE l.crawl_id = p.crawl_id AND l.from_path = p.path)::int AS out_links
FROM seo_crawl_pages p
WHERE p.crawl_id = @crawl_id;

-- name: SeoCrawlLinks :many
SELECT from_path, to_path, nofollow, occurrences FROM seo_crawl_links WHERE crawl_id = @crawl_id;

-- Every fetched page with the on-page fields an audit needs - bounded by the crawl's page budget, so the caller
-- can group and flag in one pass instead of paging
-- name: SeoCrawlAudit :many
SELECT p.path, p.status_code, p.title, p.meta_description, p.robots_meta, p.headings, p.word_count, p.kind,
       p.internal_links, p.external_links, p.noindex, p.nofollow, p.canonical, p.redirect_to, p.depth,
       p.entity_type, p.entity_id, p.fetch_ms, p.byte_size,
       (SELECT count(*) FROM seo_crawl_links l WHERE l.crawl_id = p.crawl_id AND l.to_path = p.path)::int AS in_links
FROM seo_crawl_pages p
WHERE p.crawl_id = @crawl_id;

-- The completed crawl immediately before a given one, which is what a diff compares against
-- name: PreviousSeoCrawl :one
SELECT * FROM seo_crawls
WHERE channel_id = @channel_id AND status = 'completed' AND started_at < @before
ORDER BY started_at DESC LIMIT 1;

-- name: SeoCrawlDiffRows :many
SELECT p.path, p.status_code, p.title, p.meta_description, p.canonical, p.noindex, p.depth, p.kind,
       (SELECT count(*) FROM seo_crawl_links l WHERE l.crawl_id = p.crawl_id AND l.to_path = p.path)::int AS in_links,
       COALESCE((SELECT h.value ->> 'text' FROM jsonb_array_elements(p.headings) h
                 WHERE (h.value ->> 'level') = '1' LIMIT 1), '')::text AS h1
FROM seo_crawl_pages p
WHERE p.crawl_id = @crawl_id;
