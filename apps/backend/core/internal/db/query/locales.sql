-- name: CreateLocale :one
INSERT INTO locales (
    code, name, native_name, is_active, is_rtl, sort_order,
    date_format, time_format, decimal_separator, thousands_separator,
    first_day_of_week,
    default_country_code, flag_emoji, hreflang_value,
    script, plural_rule, email_from_name_override,
    description, metadata
) VALUES (
    LOWER($1), $2, $3, $4, $5, $6,
    $7, $8, $9, $10,
    $11,
    NULLIF(UPPER($12), ''), $13, LOWER($14),
    $15, $16, $17,
    $18, $19
)
RETURNING *;

-- name: GetLocaleByCode :one
SELECT * FROM locales WHERE code = LOWER($1);

-- name: ListLocales :many
SELECT * FROM locales ORDER BY sort_order, code;

-- name: ListActiveLocales :many
SELECT * FROM locales WHERE is_active ORDER BY sort_order, code;

-- name: UpdateLocale :one
-- One statement instead of read-merge-write: an unset field keeps its value, and the prior is_active comes back so the caller can tell a real flip
WITH prior AS (
    SELECT code, is_active FROM locales WHERE code = LOWER(sqlc.arg(code)) FOR UPDATE
)
UPDATE locales l
SET name                     = COALESCE(sqlc.narg(name), l.name),
    native_name              = COALESCE(sqlc.narg(native_name), l.native_name),
    is_active                = COALESCE(sqlc.narg(is_active), l.is_active),
    is_rtl                   = COALESCE(sqlc.narg(is_rtl), l.is_rtl),
    sort_order               = COALESCE(sqlc.narg(sort_order), l.sort_order),
    date_format              = COALESCE(sqlc.narg(date_format), l.date_format),
    time_format              = COALESCE(sqlc.narg(time_format), l.time_format),
    decimal_separator        = COALESCE(sqlc.narg(decimal_separator), l.decimal_separator),
    thousands_separator      = COALESCE(sqlc.narg(thousands_separator), l.thousands_separator),
    first_day_of_week        = COALESCE(sqlc.narg(first_day_of_week), l.first_day_of_week),
    -- Blank clears to NULL, since '' violates the countries FK
    default_country_code     = CASE WHEN sqlc.arg(set_country)::bool
                                    THEN NULLIF(UPPER(sqlc.narg(default_country_code)::text), '')
                                    ELSE l.default_country_code END,
    flag_emoji               = COALESCE(sqlc.narg(flag_emoji), l.flag_emoji),
    hreflang_value           = COALESCE(LOWER(sqlc.narg(hreflang_value)::text), l.hreflang_value),
    script                   = COALESCE(sqlc.narg(script), l.script),
    plural_rule              = COALESCE(sqlc.narg(plural_rule), l.plural_rule),
    email_from_name_override = COALESCE(sqlc.narg(email_from_name_override), l.email_from_name_override),
    description              = COALESCE(sqlc.narg(description), l.description),
    metadata                 = COALESCE(sqlc.narg(metadata)::jsonb, l.metadata),
    updated_at               = NOW()
FROM prior
WHERE l.code = prior.code
RETURNING sqlc.embed(l), prior.is_active AS was_active;

-- name: DeleteLocale :exec
DELETE FROM locales WHERE code = LOWER($1);

-- name: CountLocaleReferences :one
-- Every column holding authored content or configuration in a locale - the FK tables would refuse the delete with a raw violation, and the rest (blog, CMS, menus, promotions) were orphaned silently
-- Reviews, page comments and page revisions are left out on purpose: they record what language something was written in, and counting them would make a locale undeletable forever
-- locale_references_test.go pins this list against the schema
SELECT (
    (SELECT COUNT(*) FROM channels                                WHERE default_locale = LOWER($1)) +
    (SELECT COUNT(*) FROM channel_locales                         WHERE locale_code = LOWER($1)) +
    (SELECT COUNT(*) FROM config_values                           WHERE locale = LOWER($1)) +
    (SELECT COUNT(*) FROM admin_role_permission_scopes            WHERE locale = LOWER($1)) +
    (SELECT COUNT(*) FROM catalog_attribute_content               WHERE locale = LOWER($1)) +
    (SELECT COUNT(*) FROM catalog_attribute_value_content         WHERE locale = LOWER($1)) +
    (SELECT COUNT(*) FROM catalog_category_content                WHERE locale = LOWER($1)) +
    (SELECT COUNT(*) FROM catalog_collection_content              WHERE locale = LOWER($1)) +
    (SELECT COUNT(*) FROM catalog_product_attribute_locale_values WHERE locale = LOWER($1)) +
    (SELECT COUNT(*) FROM catalog_product_bundle_option_content   WHERE locale = LOWER($1)) +
    (SELECT COUNT(*) FROM catalog_product_content                 WHERE locale = LOWER($1)) +
    (SELECT COUNT(*) FROM catalog_product_option_content          WHERE locale = LOWER($1)) +
    (SELECT COUNT(*) FROM catalog_product_option_value_content    WHERE locale = LOWER($1)) +
    (SELECT COUNT(*) FROM blog_author_content                     WHERE locale = LOWER($1)) +
    (SELECT COUNT(*) FROM blog_category_content                   WHERE locale = LOWER($1)) +
    (SELECT COUNT(*) FROM blog_post_content                       WHERE locale = LOWER($1)) +
    (SELECT COUNT(*) FROM blog_tag_content                        WHERE locale = LOWER($1)) +
    (SELECT COUNT(*) FROM cms_attribute_content                   WHERE locale = LOWER($1)) +
    (SELECT COUNT(*) FROM cms_attribute_group_content             WHERE locale = LOWER($1)) +
    (SELECT COUNT(*) FROM cms_attribute_value_content             WHERE locale = LOWER($1)) +
    (SELECT COUNT(*) FROM cms_page_content                        WHERE locale = LOWER($1)) +
    (SELECT COUNT(*) FROM cms_page_type_content                   WHERE locale = LOWER($1)) +
    (SELECT COUNT(*) FROM cms_page_variant_content                WHERE locale = LOWER($1)) +
    (SELECT COUNT(*) FROM menu_item_translations                  WHERE locale = LOWER($1)) +
    (SELECT COUNT(*) FROM promotion_rule_contents                 WHERE locale = LOWER($1)) +
    (SELECT COUNT(*) FROM review_question_content                 WHERE locale = LOWER($1))
)::bigint AS total;
