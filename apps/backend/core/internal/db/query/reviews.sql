-- ==================== Review Questions ====================

-- name: CreateReviewQuestion :one
INSERT INTO review_questions (
    slug, input_type, is_required, allow_comment, sort_order
) VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetReviewQuestionByID :one
SELECT * FROM review_questions WHERE id = $1;

-- GetReviewQuestionsByIDs is the batched form for the storefront DataLoader - every review-answer `question` resolver across a page collapses into one query for the distinct question ids referenced by the rendered reviews
-- name: GetReviewQuestionsByIDs :many
SELECT * FROM review_questions WHERE id = ANY(sqlc.arg(ids)::uuid[]);

-- name: GetReviewQuestionBySlug :one
SELECT * FROM review_questions WHERE slug = $1;

-- name: ListReviewQuestions :many
SELECT * FROM review_questions ORDER BY sort_order, slug;

-- name: UpdateReviewQuestion :one
UPDATE review_questions
SET is_required   = $2,
    allow_comment = $3,
    sort_order    = $4,
    updated_at    = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteReviewQuestion :exec
DELETE FROM review_questions WHERE id = $1;

-- name: UpsertReviewQuestionContent :one
INSERT INTO review_question_content (question_id, locale, label, help_text)
VALUES ($1, $2, $3, $4)
ON CONFLICT (question_id, locale) DO UPDATE
SET label = EXCLUDED.label, help_text = EXCLUDED.help_text, updated_at = NOW()
RETURNING *;

-- name: ListReviewQuestionContent :many
SELECT * FROM review_question_content WHERE question_id = $1 ORDER BY locale;

-- name: ListReviewQuestionContentByIDs :many
SELECT * FROM review_question_content
WHERE question_id = ANY(sqlc.arg(question_ids)::uuid[]) AND locale = $1;

-- ==================== Question Assignments (per Product Type) ====================

-- name: AssignReviewQuestion :exec
INSERT INTO review_question_assignments (product_type_id, question_id, sort_order)
VALUES ($1, $2, $3)
ON CONFLICT (product_type_id, question_id) DO UPDATE
SET sort_order = EXCLUDED.sort_order;

-- name: UnassignReviewQuestion :execrows
DELETE FROM review_question_assignments
WHERE product_type_id = $1 AND question_id = $2;

-- ListAssignedQuestions returns the curated set of questions for a given product type, sorted by the assignment's sort_order then question slug Joins the question row so callers don't N+1
-- name: ListAssignedQuestions :many
SELECT q.*, a.sort_order AS assignment_sort_order
FROM review_question_assignments a
JOIN review_questions q ON q.id = a.question_id
WHERE a.product_type_id = $1
ORDER BY a.sort_order, q.slug;

-- ==================== Reviews ====================

-- name: CreateReview :one
-- customer_id is nullable: guest authors submit with guest_name (+ optional guest_email) instead - the reviews_author_present_chk constraint guarantees exactly one identity is set, so callers supplying customer_id leave guest_name empty and vice versa
INSERT INTO reviews (
    product_id, customer_id, channel_id, locale,
    title, body, is_verified_purchase, status,
    guest_name, guest_email
) VALUES (
    $1, sqlc.narg('customer_id')::uuid, $2, $3,
    $4, $5, $6, $7,
    $8, $9
)
RETURNING *;

-- UpdateReview replaces the editable fields of an existing review - the customer edits their own review; on edit we re-set status to pending so an admin re-moderates the changed content
-- name: UpdateReview :one
UPDATE reviews
SET title  = $2,
    body   = $3,
    status = 'pending',
    moderated_at   = NULL,
    moderated_by   = NULL,
    moderated_by_integration_id = NULL,
    moderator_note = ''
WHERE id = $1
RETURNING *;

-- name: GetReviewByID :one
SELECT * FROM reviews WHERE id = $1;

-- name: GetReviewByCustomerAndProduct :one
SELECT * FROM reviews
WHERE customer_id = $1 AND product_id = $2;

-- ListApprovedReviewsForProduct returns the public review list - the storefront calls this for the PDP reviews section - sort is decided in the resolver layer by composing a CASE; this query orders newest- first as the default and the resolver re-orders the slice when a different sort is requested (only 10–50 rows typically - cheap)
-- name: ListApprovedReviewsForProduct :many
SELECT r.* FROM reviews r
WHERE r.product_id = sqlc.arg(product_id) AND r.status = 'approved' AND (sqlc.narg(channel_id)::uuid IS NULL OR r.channel_id = sqlc.narg(channel_id)::uuid)
ORDER BY r.submitted_at DESC
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: CountApprovedReviewsForProduct :one
SELECT COUNT(*) FROM reviews r
WHERE r.product_id = sqlc.arg(product_id) AND r.status = 'approved' AND (sqlc.narg(channel_id)::uuid IS NULL OR r.channel_id = sqlc.narg(channel_id)::uuid);

-- name: ListReviewChannelsForProduct :many
SELECT DISTINCT channel_id FROM reviews WHERE product_id = $1 AND status = 'approved';

-- ListApprovedReviewsForProductIDs is the batched form of ListApprovedReviewsForProduct: it returns the top-N approved reviews for many products in a single windowed query, so the storefront can hydrate the reviews of a configurable's children (or any product list) without one query per product - ROW_NUMBER enforces the same newest-first per-product window the single-product query applies via LIMIT/OFFSET; the resolver re-sorts in memory when a different sort is asked for, exactly as the single-product path does
-- name: ListApprovedReviewsForProductIDs :many
SELECT id, product_id, customer_id, guest_name, guest_email, channel_id,
       locale, title, body, is_verified_purchase, status, submitted_at,
       moderated_at, moderated_by, moderator_note, merchant_reply, merchant_reply_at,
       merchant_reply_by, moderation_label, moderation_reason, moderation_checked_at
FROM (
  SELECT r.*, ROW_NUMBER() OVER (
           PARTITION BY r.product_id ORDER BY r.submitted_at DESC
         ) AS rn
  FROM reviews r
  WHERE r.product_id = ANY(sqlc.arg(product_ids)::uuid[])
    AND r.status = 'approved'
    AND (sqlc.narg(channel_id)::uuid IS NULL OR r.channel_id = sqlc.narg(channel_id)::uuid)
) t
WHERE t.rn > sqlc.arg(row_offset)::int
  AND t.rn <= (sqlc.arg(row_offset)::int + sqlc.arg(row_limit)::int)
ORDER BY t.product_id, t.submitted_at DESC;

-- Reviews for moderation, filterable by status, product and channel, newest first so the freshest pending review tops the queue
-- The channel_ids narg scopes the page to the admin's permitted channels so pagination and total stay correct, and a NULL array no-ops the filter for bypass admins
-- name: ListReviewsForModeration :many
SELECT * FROM reviews
WHERE
    (sqlc.narg('status_filter')::text[] IS NULL
     OR status = ANY(sqlc.narg('status_filter')::text[]))
AND (sqlc.narg('product_id')::uuid IS NULL
     OR product_id = sqlc.narg('product_id')::uuid)
AND (sqlc.narg('channel_ids')::uuid[] IS NULL
     OR channel_id = ANY(sqlc.narg('channel_ids')::uuid[]))
AND (sqlc.narg('category_slugs')::text[] IS NULL
     OR jsonb_exists_any(category_slugs, sqlc.narg('category_slugs')::text[]))
ORDER BY submitted_at DESC
LIMIT $1 OFFSET $2;

-- name: CountReviewsForModeration :one
SELECT COUNT(*) FROM reviews
WHERE
    (sqlc.narg('status_filter')::text[] IS NULL
     OR status = ANY(sqlc.narg('status_filter')::text[]))
AND (sqlc.narg('product_id')::uuid IS NULL
     OR product_id = sqlc.narg('product_id')::uuid)
AND (sqlc.narg('channel_ids')::uuid[] IS NULL
     OR channel_id = ANY(sqlc.narg('channel_ids')::uuid[]))
AND (sqlc.narg('category_slugs')::text[] IS NULL
     OR jsonb_exists_any(category_slugs, sqlc.narg('category_slugs')::text[]));

-- CountReviewsByStatus returns the per-status review tallies for the hub tiles (pending/approved/rejected), scoped to the admin's permitted channels and an optional product - exact regardless of pagination, so the tiles never drift from the table
-- name: CountReviewsByStatus :many
SELECT status, COUNT(*) AS count FROM reviews
WHERE
    (sqlc.narg('product_id')::uuid IS NULL
     OR product_id = sqlc.narg('product_id')::uuid)
AND (sqlc.narg('channel_ids')::uuid[] IS NULL
     OR channel_id = ANY(sqlc.narg('channel_ids')::uuid[]))
GROUP BY status;

-- name: SetReviewStatus :one
UPDATE reviews
SET status         = $2,
    moderated_at   = NOW(),
    moderated_by   = $3,
    moderator_note = $4,
    moderated_by_integration_id = $5
WHERE id = $1
RETURNING *;

-- name: SetReviewModeration :one
-- Stores the AI classification verdict: a derived coarse label + reason, the applicable categories ($4, a jsonb array of category slugs), and the moderation flags ($5, a jsonb array of tag slugs) - status untouched - advisory
UPDATE reviews
SET moderation_label      = $2,
    moderation_reason     = $3,
    category_slugs        = $4,
    moderation_tags       = $5,
    moderation_checked_at = NOW()
WHERE id = $1
RETURNING *;

-- name: SetReviewCategories :one
-- Manual category override by a moderator (a jsonb array of slugs; '[]' clears)
UPDATE reviews
SET category_slugs = $2
WHERE id = $1
RETURNING *;

-- name: SetReviewModerationTags :one
-- Manual moderation-tag override by a moderator (a jsonb array of slugs; '[]'
-- clears all) - independent of the AI label/reason
UPDATE reviews
SET moderation_tags = $2
WHERE id = $1
RETURNING *;

-- name: SetReviewMerchantReply :one
UPDATE reviews
SET merchant_reply    = sqlc.arg(reply)::text,
    merchant_reply_at = NOW(),
    merchant_reply_by = sqlc.narg(admin_id)
WHERE id = $1
RETURNING *;

-- name: ClearReviewMerchantReply :one
UPDATE reviews
SET merchant_reply    = '',
    merchant_reply_at = NULL,
    merchant_reply_by = NULL
WHERE id = $1
RETURNING *;

-- ==================== Product review summaries (AI "what customers say") ====================

-- name: GetProductReviewSummary :one
SELECT * FROM product_review_summaries WHERE product_id = $1 AND channel_id = $2;

-- name: UpsertProductReviewSummary :exec
INSERT INTO product_review_summaries (product_id, channel_id, summary, review_count, is_stale, generated_at, updated_at)
VALUES ($1, $2, $3, $4, FALSE, NOW(), NOW())
ON CONFLICT (product_id, channel_id) DO UPDATE
SET summary      = EXCLUDED.summary,
    review_count = EXCLUDED.review_count,
    is_stale     = FALSE,
    generated_at = NOW(),
    updated_at   = NOW();

-- name: MarkProductReviewSummaryStale :exec
INSERT INTO product_review_summaries (product_id, channel_id, is_stale, updated_at)
VALUES ($1, $2, TRUE, NOW())
ON CONFLICT (product_id, channel_id) DO UPDATE
SET is_stale = TRUE, updated_at = NOW();

-- name: MarkProductReviewSummaryFresh :exec
UPDATE product_review_summaries SET is_stale = FALSE, updated_at = NOW() WHERE product_id = $1 AND channel_id = $2;

-- name: ListStaleProductReviewSummaryIDs :many
SELECT product_id, channel_id FROM product_review_summaries WHERE is_stale ORDER BY updated_at LIMIT $1;

-- ==================== Review Answers ====================

-- name: UpsertReviewAnswer :exec
INSERT INTO review_answers (review_id, question_id, rating_value, bool_value, text_value)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (review_id, question_id) DO UPDATE
SET rating_value = EXCLUDED.rating_value,
    bool_value   = EXCLUDED.bool_value,
    text_value   = EXCLUDED.text_value;

-- name: DeleteReviewAnswersForReview :exec
DELETE FROM review_answers WHERE review_id = $1;

-- name: ListReviewAnswersForReview :many
SELECT * FROM review_answers WHERE review_id = $1;

-- name: ListReviewAnswersByReviewIDs :many
SELECT * FROM review_answers
WHERE review_id = ANY(sqlc.arg(review_ids)::uuid[]);

-- ==================== Review Media ====================

-- name: InsertReviewMedia :one
INSERT INTO review_media (review_id, asset_id, sort_order)
VALUES ($1, $2, $3)
ON CONFLICT (review_id, asset_id) DO UPDATE
SET sort_order = EXCLUDED.sort_order
RETURNING *;

-- name: DeleteReviewMediaForReview :exec
DELETE FROM review_media WHERE review_id = $1;

-- name: ListReviewMediaForReview :many
SELECT rm.*, ma.object_key, ma.mime_type, ma.width, ma.height, ma.size_bytes
FROM review_media rm
JOIN media_assets ma ON ma.id = rm.asset_id
WHERE rm.review_id = $1
ORDER BY rm.sort_order, rm.created_at;

-- name: ListReviewMediaByReviewIDs :many
SELECT rm.*, ma.object_key, ma.mime_type, ma.width, ma.height, ma.size_bytes
FROM review_media rm
JOIN media_assets ma ON ma.id = rm.asset_id
WHERE rm.review_id = ANY(sqlc.arg(review_ids)::uuid[])
ORDER BY rm.review_id, rm.sort_order;

-- ==================== Aggregates ====================

-- AggregateReviewRatings returns one row per (product, question) with the mean rating and count over approved reviews - the storefront per-question average + overall count are computed from this set
-- name: AggregateReviewRatings :many
SELECT
    r.product_id,
    a.question_id,
    AVG(a.rating_value)::FLOAT AS average,
    COUNT(*)::INT             AS sample_count
FROM review_answers a
JOIN reviews r ON r.id = a.review_id
WHERE r.product_id = sqlc.arg(product_id)
  AND r.status = 'approved'
  AND a.rating_value IS NOT NULL
  AND (sqlc.narg(channel_id)::uuid IS NULL OR r.channel_id = sqlc.narg(channel_id)::uuid)
GROUP BY r.product_id, a.question_id;

-- AggregateOverallRating returns count + overall mean + 5-bucket distribution for the PDP summary card
-- name: AggregateOverallRating :one
WITH per_review AS (
    SELECT
        r.id,
        AVG(a.rating_value)::FLOAT AS review_avg
    FROM reviews r
    JOIN review_answers a ON a.review_id = r.id
    WHERE r.product_id = sqlc.arg(product_id)
      AND r.status = 'approved'
      AND a.rating_value IS NOT NULL
      AND (sqlc.narg(channel_id)::uuid IS NULL OR r.channel_id = sqlc.narg(channel_id)::uuid)
    GROUP BY r.id
)
SELECT
    COUNT(*)::INT AS review_count,
    COALESCE(AVG(review_avg)::FLOAT, 0)::FLOAT AS overall_average,
    COUNT(*) FILTER (WHERE review_avg <  1.5)::INT AS bucket_1,
    COUNT(*) FILTER (WHERE review_avg >= 1.5 AND review_avg < 2.5)::INT AS bucket_2,
    COUNT(*) FILTER (WHERE review_avg >= 2.5 AND review_avg < 3.5)::INT AS bucket_3,
    COUNT(*) FILTER (WHERE review_avg >= 3.5 AND review_avg < 4.5)::INT AS bucket_4,
    COUNT(*) FILTER (WHERE review_avg >= 4.5)::INT AS bucket_5
FROM per_review;

-- AggregateOverallRatingsByProductIDs is the batched form - used by the storefront PLP path to hydrate review summaries for many products in one query instead of one per card - same per-review averaging rules as the single-product form above
-- name: AggregateOverallRatingsByProductIDs :many
WITH per_review AS (
    SELECT
        r.product_id,
        r.id,
        AVG(a.rating_value)::FLOAT AS review_avg
    FROM reviews r
    JOIN review_answers a ON a.review_id = r.id
    WHERE r.product_id = ANY(sqlc.arg(product_ids)::uuid[])
      AND r.status = 'approved'
      AND a.rating_value IS NOT NULL
      AND (sqlc.narg(channel_id)::uuid IS NULL OR r.channel_id = sqlc.narg(channel_id)::uuid)
    GROUP BY r.product_id, r.id
)
SELECT
    product_id,
    COUNT(*)::INT AS review_count,
    COALESCE(AVG(review_avg)::FLOAT, 0)::FLOAT AS overall_average,
    COUNT(*) FILTER (WHERE review_avg <  1.5)::INT AS bucket_1,
    COUNT(*) FILTER (WHERE review_avg >= 1.5 AND review_avg < 2.5)::INT AS bucket_2,
    COUNT(*) FILTER (WHERE review_avg >= 2.5 AND review_avg < 3.5)::INT AS bucket_3,
    COUNT(*) FILTER (WHERE review_avg >= 3.5 AND review_avg < 4.5)::INT AS bucket_4,
    COUNT(*) FILTER (WHERE review_avg >= 4.5)::INT AS bucket_5
FROM per_review
GROUP BY product_id;

-- AggregateReviewRatingsByProductIDs is the per-question variant of
-- AggregateReviewRatings, batched by product id
-- name: AggregateReviewRatingsByProductIDs :many
SELECT
    r.product_id,
    a.question_id,
    AVG(a.rating_value)::FLOAT AS average,
    COUNT(*)::INT             AS sample_count
FROM review_answers a
JOIN reviews r ON r.id = a.review_id
WHERE r.product_id = ANY(sqlc.arg(product_ids)::uuid[])
  AND r.status = 'approved'
  AND a.rating_value IS NOT NULL
  AND (sqlc.narg(channel_id)::uuid IS NULL OR r.channel_id = sqlc.narg(channel_id)::uuid)
GROUP BY r.product_id, a.question_id;

-- ==================== Review Votes (helpful / not-helpful) ====================

-- UpsertReviewVote* - one per identity type so the unique partial index can do its job - customers and anon voters
-- cannot collide, so the two queries never see each other's rows

-- name: UpsertReviewVoteByCustomer :exec
INSERT INTO review_votes (review_id, customer_id, value)
VALUES ($1, $2, $3)
ON CONFLICT (review_id, customer_id) WHERE customer_id IS NOT NULL DO UPDATE
SET value = EXCLUDED.value, updated_at = NOW();

-- name: UpsertReviewVoteByAnon :exec
INSERT INTO review_votes (review_id, voter_token, value)
VALUES ($1, $2, $3)
ON CONFLICT (review_id, voter_token) WHERE voter_token IS NOT NULL DO UPDATE
SET value = EXCLUDED.value, updated_at = NOW();

-- name: DeleteReviewVoteByCustomer :execrows
DELETE FROM review_votes WHERE review_id = $1 AND customer_id = $2;

-- name: DeleteReviewVoteByAnon :execrows
DELETE FROM review_votes WHERE review_id = $1 AND voter_token = $2;

-- AggregateReviewVotesByReviewIDs returns helpful + not-helpful counts per review, batched so the storefront review list hydrates votes in one query rather than N - counts every row regardless of identity type so customer + anon votes contribute to the same tally
-- name: AggregateReviewVotesByReviewIDs :many
SELECT
    review_id,
    COUNT(*) FILTER (WHERE value =  1)::INT AS helpful_count,
    COUNT(*) FILTER (WHERE value = -1)::INT AS not_helpful_count
FROM review_votes
WHERE review_id = ANY(sqlc.arg(review_ids)::uuid[])
GROUP BY review_id;

-- GetMyReviewVotesByCustomer returns the signed-in customer's own vote (+1/-1) on each of the given reviews - empty rows for reviews the customer hasn't voted on - used to highlight the active button
-- name: GetMyReviewVotesByCustomer :many
SELECT review_id, value
FROM review_votes
WHERE customer_id = $1
  AND review_id = ANY(sqlc.arg(review_ids)::uuid[]);

-- GetMyReviewVotesByAnon is the anonymous variant - keyed on the
-- voter token from the storefront's review-voter cookie
-- name: GetMyReviewVotesByAnon :many
SELECT review_id, value
FROM review_votes
WHERE voter_token = $1
  AND review_id = ANY(sqlc.arg(review_ids)::uuid[]);

-- ==================== Verified-purchase check ====================

-- HasCustomerPurchasedProduct: returns 1 row if the customer has at least one confirmed/fulfilled order line for the product's SKU, 0 rows otherwise - used at review submission to set is_verified_purchase and to gate the customerCanReview eligibility resolver
-- name: HasCustomerPurchasedProduct :one
SELECT EXISTS (
    SELECT 1
    FROM order_lines ol
    JOIN orders o ON o.id = ol.order_id
    WHERE o.customer_id = $1
      AND o.status IN ('confirmed', 'fulfilled')
      AND ol.product_sku = (SELECT sku FROM catalog_products p WHERE p.id = $2)
) AS purchased;
