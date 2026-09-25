-- ===== Rules =====

-- name: CreatePromotionRule :one
INSERT INTO promotion_rules (
    slug, name, description, public_label,
    channel_id, requires_code, priority, stop_after_apply,
    exclusivity_group, valid_from, valid_to, schedule_json,
    max_uses, per_customer_limit, is_active,
    created_by_admin_id, updated_by_admin_id,
    label_media_id, banner_media_id,
    pill_text, pill_text_color, pill_bg_color
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15,
    $16, $17, $18, $19, $20, $21, $22
)
RETURNING *;

-- name: UpdatePromotionRule :one
UPDATE promotion_rules
SET slug                = $2,
    name                = $3,
    description         = $4,
    public_label    = $5,
    channel_id       = $6,
    requires_code       = $7,
    priority            = $8,
    stop_after_apply    = $9,
    exclusivity_group   = $10,
    valid_from          = $11,
    valid_to            = $12,
    schedule_json       = $13,
    max_uses            = $14,
    per_customer_limit  = $15,
    is_active           = $16,
    updated_by_admin_id = $17,
    label_media_id      = $18,
    banner_media_id     = $19,
    pill_text           = $20,
    pill_text_color     = $21,
    pill_bg_color       = $22,
    updated_at          = NOW()
WHERE id = $1
RETURNING *;

-- name: DeletePromotionRule :exec
DELETE FROM promotion_rules WHERE id = $1;

-- name: GetPromotionRule :one
SELECT * FROM promotion_rules WHERE id = $1;

-- name: GetPromotionRuleBySlug :one
SELECT * FROM promotion_rules WHERE slug = $1;

-- name: ListPromotionRules :many
SELECT * FROM promotion_rules
ORDER BY priority, created_at DESC
LIMIT $1 OFFSET $2;

-- name: CountPromotionRules :one
SELECT COUNT(*)::INTEGER FROM promotion_rules;

-- name: ListActiveRulesForChannel :many
-- Loaded by the engine - NULL channel_id matches everything; otherwise must match the request's store The max_uses cap is enforced here, not just at redemption - a used-up rule that still fires discounts the cart and then fails placement, with no applied-code row to remove when the rule auto-applies
SELECT *
FROM promotion_rules
WHERE is_active = TRUE
  AND (valid_from IS NULL OR valid_from <= NOW())
  AND (valid_to   IS NULL OR valid_to   >= NOW())
  AND (max_uses IS NULL OR used_count < max_uses)
  AND (channel_id IS NULL OR channel_id = $1)
ORDER BY priority, created_at;

-- name: IncrementRuleUsage :execrows
-- Cap-guarded: refuses the increment once used_count has reached max_uses, so concurrent checkouts can't overshoot the cap (the ApplyCode-time check is read-then-act and races) - zero rows affected means the cap was hit; the repository maps that to ErrRuleMaxUses - the UPDATE also row-locks the rule, serializing the per-customer re-checks that follow in Redeem
UPDATE promotion_rules SET used_count = used_count + 1, updated_at = NOW()
WHERE id = $1 AND (max_uses IS NULL OR used_count < max_uses);

-- name: ReorderPromotionRules :exec
-- Batch-update priorities by position in the ordered_ids array - output priority is `position * 10` (10, 20, 30 …) so future single-row inserts can land between two rules without a full renumber (admin can type priority=15 to slot between positions 1 and 2) - rules not included in the array keep their existing priority - drag-and-drop only reorders the visible page
UPDATE promotion_rules
SET priority = sub.new_priority, updated_at = NOW()
FROM (
    SELECT id, (ord * 10)::INTEGER AS new_priority
    FROM unnest(sqlc.arg(ordered_ids)::UUID[]) WITH ORDINALITY AS t(id, ord)
) sub
WHERE promotion_rules.id = sub.id;

-- ===== Codes =====

-- name: CreatePromotionCode :one
INSERT INTO promotion_codes (
    rule_id, code, max_uses, per_customer_limit,
    valid_from, valid_to, restricted_to_email, is_active
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: UpdatePromotionCode :one
UPDATE promotion_codes
SET code                = $2,
    max_uses            = $3,
    per_customer_limit  = $4,
    valid_from          = $5,
    valid_to            = $6,
    restricted_to_email = $7,
    is_active           = $8,
    updated_at          = NOW()
WHERE id = $1
RETURNING *;

-- name: DeletePromotionCode :exec
DELETE FROM promotion_codes WHERE id = $1;

-- name: GetPromotionCode :one
SELECT * FROM promotion_codes WHERE id = $1;

-- name: GetPromotionCodeByCode :one
-- Hot path for applyCoupon - returns the code row OR no rows
SELECT * FROM promotion_codes
WHERE code = $1 AND is_active = TRUE;

-- name: ListPromotionCodesByRule :many
SELECT * FROM promotion_codes
WHERE rule_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: CountPromotionCodesByRule :one
SELECT COUNT(*)::INTEGER FROM promotion_codes WHERE rule_id = $1;

-- name: IncrementCodeUsage :execrows
-- Cap-guarded like IncrementRuleUsage: zero rows affected = cap hit
-- (mapped to ErrCodeMaxUses by the repository)
UPDATE promotion_codes SET used_count = used_count + 1, updated_at = NOW()
WHERE id = $1 AND (max_uses IS NULL OR used_count < max_uses);

-- name: CountCodeUsageByCustomer :one
SELECT COUNT(*)::INTEGER FROM promotion_redemptions
WHERE code_id = $1 AND customer_id = $2;

-- name: CountRuleUsageByCustomer :one
SELECT COUNT(*)::INTEGER FROM promotion_redemptions
WHERE rule_id = $1 AND customer_id = $2;

-- name: BulkInsertPromotionCodes :execrows
-- Used by the batch-code generator - codes are passed as parallel arrays so a 10k-code batch fits in one round-trip - ON CONFLICT skips dupes - the generator regenerates collisions and retries
INSERT INTO promotion_codes (rule_id, code)
SELECT $1, unnest(sqlc.arg(codes)::TEXT[])
ON CONFLICT (code) DO NOTHING;

-- ===== Conditions =====

-- name: CreatePromotionCondition :one
INSERT INTO promotion_conditions (
    rule_id, type, params, params_version, sort_order
) VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: DeleteRuleConditions :exec
-- Used by the admin update path to clear before re-inserting
DELETE FROM promotion_conditions WHERE rule_id = $1;

-- name: ListConditionsByRule :many
SELECT * FROM promotion_conditions
WHERE rule_id = $1
ORDER BY sort_order, created_at;

-- name: ListConditionsByRules :many
-- Engine hot path: load all conditions for all candidate rules in
-- one query rather than N+1
SELECT * FROM promotion_conditions
WHERE rule_id = ANY(sqlc.arg(rule_ids)::UUID[])
ORDER BY rule_id, sort_order;

-- ===== Actions =====

-- name: CreatePromotionAction :one
INSERT INTO promotion_actions (
    rule_id, type, params, params_version, sort_order
) VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: DeleteRuleActions :exec
DELETE FROM promotion_actions WHERE rule_id = $1;

-- name: ListActionsByRule :many
SELECT * FROM promotion_actions
WHERE rule_id = $1
ORDER BY sort_order, created_at;

-- name: ListActionsByRules :many
SELECT * FROM promotion_actions
WHERE rule_id = ANY(sqlc.arg(rule_ids)::UUID[])
ORDER BY rule_id, sort_order;

-- ===== Cart applied codes =====

-- name: ApplyCodeToCart :one
INSERT INTO cart_applied_codes (cart_id, rule_id, code_id)
VALUES ($1, $2, $3)
ON CONFLICT (cart_id, rule_id) DO UPDATE
SET code_id    = EXCLUDED.code_id,
    applied_at = NOW(),
    invalidated_reason = NULL
RETURNING *;

-- name: RemoveCodeFromCartByCode :exec
-- Storefront removeCoupon takes a code (not an internal rule id)
-- Find the rule via the code join and delete the cart row
DELETE FROM cart_applied_codes
WHERE cart_id = $1
  AND code_id IN (SELECT id FROM promotion_codes WHERE code = $2);

-- name: RemoveCodeFromCartByRule :exec
DELETE FROM cart_applied_codes WHERE cart_id = $1 AND rule_id = $2;

-- name: ClearCartAppliedCodes :exec
DELETE FROM cart_applied_codes WHERE cart_id = $1;

-- name: ListCartAppliedCodes :many
SELECT cac.*, pc.code AS code_text
FROM cart_applied_codes cac
LEFT JOIN promotion_codes pc ON pc.id = cac.code_id
WHERE cac.cart_id = $1;

-- name: MarkCartCodeInvalidated :exec
UPDATE cart_applied_codes
SET invalidated_reason = $3
WHERE cart_id = $1 AND rule_id = $2;

-- name: GetCartEmailForPromotions :one
-- Backs the promotion_codes.restricted_to_email gate - the checkout email typed onto the cart wins; a logged-in shopper who hasn't typed one yet falls back to the account address
SELECT COALESCE(NULLIF(c.email, ''), cu.email, '')::TEXT AS email
FROM carts c
LEFT JOIN customers cu ON cu.id = c.customer_id
WHERE c.id = $1;

-- ===== Redemptions =====

-- name: RecordPromotionRedemption :exec
INSERT INTO promotion_redemptions (
    rule_id, code_id, customer_id, order_id, discount_cents
) VALUES ($1, $2, $3, $4, $5);

-- ===== Analytics ===== Backs the /promotions/analytics dashboard page - one row per rule with aggregate counts, discount totals, distinct customer reach, and the summed order revenue those redemptions appeared on - range filters via start/end timestamps; pass start = '0001-01-01', end = NOW() for "all time"

-- name: PromotionAnalyticsByRule :many
SELECT
    r.id                                              AS rule_id,
    r.slug                                            AS rule_slug,
    r.name                                            AS rule_name,
    COUNT(rd.id)::INTEGER                             AS redemption_count,
    COALESCE(SUM(rd.discount_cents), 0)::BIGINT       AS discount_total_cents,
    COUNT(DISTINCT rd.customer_id)::INTEGER           AS unique_customers,
    COALESCE(SUM(o.total_amount), 0)::BIGINT          AS order_revenue_cents
FROM promotion_rules r
LEFT JOIN promotion_redemptions rd
       ON rd.rule_id = r.id
      AND rd.redeemed_at >= $1
      AND rd.redeemed_at <= $2
LEFT JOIN orders o ON o.id = rd.order_id
GROUP BY r.id, r.slug, r.name
ORDER BY redemption_count DESC, r.slug;

-- name: ListPromotionRedemptionsByOrder :many
-- What the promotion engine actually applied to one order, with the rule name and any code the shopper typed - orders store only an aggregate discountAmount, so without this nobody can answer why an order was discounted
SELECT
    rd.id,
    rd.discount_cents,
    rd.redeemed_at,
    rd.rule_id,
    COALESCE(r.name, '')::TEXT AS rule_name,
    COALESCE(r.slug, '')::TEXT AS rule_slug,
    COALESCE(c.code, '')::TEXT AS code
FROM promotion_redemptions rd
LEFT JOIN promotion_rules r ON r.id = rd.rule_id
LEFT JOIN promotion_codes c ON c.id = rd.code_id
WHERE rd.order_id = $1
ORDER BY rd.redeemed_at;

-- name: PromotionAnalyticsTotals :one
SELECT
    COUNT(*)::INTEGER                                 AS redemption_count,
    COALESCE(SUM(discount_cents), 0)::BIGINT          AS discount_total_cents,
    COUNT(DISTINCT customer_id)::INTEGER              AS unique_customers
FROM promotion_redemptions
WHERE redeemed_at >= $1
  AND redeemed_at <= $2;

-- name: PromotionAnalyticsDaily :many
-- Daily redemption + discount series. rule_id is optional: NULL → every rule
-- (the hub trend); set → a single promotion's trend
SELECT
    date_trunc('day', rd.redeemed_at)::timestamptz   AS day,
    COUNT(rd.id)::INTEGER                             AS redemption_count,
    COALESCE(SUM(rd.discount_cents), 0)::BIGINT       AS discount_total_cents
FROM promotion_redemptions rd
WHERE rd.redeemed_at >= sqlc.arg(start_time)
  AND rd.redeemed_at <= sqlc.arg(end_time)
  AND (sqlc.narg(rule_id)::uuid IS NULL OR rd.rule_id = sqlc.narg(rule_id)::uuid)
GROUP BY 1
ORDER BY 1;

-- ===== i18n rule contents =====

-- name: UpsertPromotionRuleContent :one
INSERT INTO promotion_rule_contents (rule_id, locale, label, terms)
VALUES ($1, $2, $3, $4)
ON CONFLICT (rule_id, locale) DO UPDATE
SET label = EXCLUDED.label,
    terms = EXCLUDED.terms,
    updated_at = NOW()
RETURNING *;

-- name: ListPromotionRuleContents :many
SELECT * FROM promotion_rule_contents
WHERE rule_id = $1
ORDER BY locale;

-- name: DeletePromotionRuleContent :exec
DELETE FROM promotion_rule_contents WHERE rule_id = $1 AND locale = $2;

-- name: GetPromotionRuleContent :one
-- Used by the storefront's locale-resolved label lookup - returns no rows when the requested locale doesn't have content; caller falls back to the rule's own public_label
SELECT * FROM promotion_rule_contents
WHERE rule_id = $1 AND locale = $2;

-- name: GetPromotionRuleContentsByIDs :many
-- DataLoader-style batched lookup - returns one row per (rule_id, locale) pair that exists; missing rules / locales are absent from the result - caller falls back to the rule's own public_label
SELECT * FROM promotion_rule_contents
WHERE rule_id = ANY(sqlc.arg(rule_ids)::uuid[])
  AND locale = sqlc.arg(locale)::text;

-- name: GetPromotionLineAttrsBySKUs :many
-- The catalog facts core.product_kind, core.product_in_category and core.product_has_attribute gate on - cart lines carry only sku, price and quantity, so without this enrichment those three conditions never matched a real cart - they only appeared to work in the admin simulator, which is fed kind and category ids by hand One round-trip for the whole cart - selectable attribute values come back as slug -> comma-joined value slugs; free-text scalars ride along in the product's own attributes bag, already keyed by attribute slug
WITH target AS (
    SELECT id, sku, kind, attributes
    FROM catalog_products
    WHERE sku = ANY(sqlc.arg(skus)::TEXT[])
), cats AS (
    SELECT pc.product_id, array_agg(pc.category_id ORDER BY pc.position) AS category_ids
    FROM catalog_product_categories pc
    JOIN target t ON t.id = pc.product_id
    GROUP BY pc.product_id
), sel_pairs AS (
    SELECT pav.product_id,
           a.slug AS attribute_slug,
           string_agg(av.slug, ',' ORDER BY pav.sort_order, av.slug) AS value_slugs
    FROM catalog_product_attribute_values pav
    JOIN target t ON t.id = pav.product_id
    JOIN catalog_attributes a ON a.id = pav.attribute_id
    JOIN catalog_attribute_values av ON av.id = pav.attribute_value_id
    GROUP BY pav.product_id, a.slug
), sel AS (
    SELECT product_id, jsonb_object_agg(attribute_slug, value_slugs) AS kv
    FROM sel_pairs
    GROUP BY product_id
)
SELECT t.id AS product_id,
       t.sku,
       t.kind::TEXT AS kind,
       COALESCE(c.category_ids, ARRAY[]::UUID[])::UUID[] AS category_ids,
       COALESCE(s.kv, '{}'::JSONB) AS selectable_attrs,
       t.attributes AS scalar_attrs
FROM target t
LEFT JOIN cats c ON c.product_id = t.id
LEFT JOIN sel  s ON s.product_id = t.id;

-- name: PromotionAnalyticsByRuleForChannels :many
SELECT
    r.id                                              AS rule_id,
    r.slug                                            AS rule_slug,
    r.name                                            AS rule_name,
    COUNT(rd.id)::INTEGER                             AS redemption_count,
    COALESCE(SUM(rd.discount_cents), 0)::BIGINT       AS discount_total_cents,
    COUNT(DISTINCT rd.customer_id)::INTEGER           AS unique_customers,
    COALESCE(SUM(o.total_amount), 0)::BIGINT          AS order_revenue_cents
FROM promotion_rules r
LEFT JOIN (
    promotion_redemptions rd
    JOIN orders o ON o.id = rd.order_id AND o.channel_id = ANY(sqlc.arg(channel_ids)::uuid[])
)      ON rd.rule_id = r.id
      AND rd.redeemed_at >= sqlc.arg(start_time)
      AND rd.redeemed_at <= sqlc.arg(end_time)
WHERE r.channel_id IS NULL OR r.channel_id = ANY(sqlc.arg(channel_ids)::uuid[])
GROUP BY r.id, r.slug, r.name
ORDER BY redemption_count DESC, r.slug;

-- name: PromotionAnalyticsTotalsForChannels :one
SELECT
    COUNT(rd.id)::INTEGER                             AS redemption_count,
    COALESCE(SUM(rd.discount_cents), 0)::BIGINT       AS discount_total_cents,
    COUNT(DISTINCT rd.customer_id)::INTEGER           AS unique_customers
FROM promotion_redemptions rd
JOIN orders o ON o.id = rd.order_id
WHERE rd.redeemed_at >= sqlc.arg(start_time)
  AND rd.redeemed_at <= sqlc.arg(end_time)
  AND o.channel_id = ANY(sqlc.arg(channel_ids)::uuid[]);

-- name: PromotionAnalyticsDailyForChannels :many
SELECT
    date_trunc('day', rd.redeemed_at)::timestamptz   AS day,
    COUNT(rd.id)::INTEGER                             AS redemption_count,
    COALESCE(SUM(rd.discount_cents), 0)::BIGINT       AS discount_total_cents
FROM promotion_redemptions rd
JOIN orders o ON o.id = rd.order_id
WHERE rd.redeemed_at >= sqlc.arg(start_time)
  AND rd.redeemed_at <= sqlc.arg(end_time)
  AND (sqlc.narg(rule_id)::uuid IS NULL OR rd.rule_id = sqlc.narg(rule_id)::uuid)
  AND o.channel_id = ANY(sqlc.arg(channel_ids)::uuid[])
GROUP BY 1
ORDER BY 1;
