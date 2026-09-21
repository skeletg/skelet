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
UPDATE locales
SET name                     = $2,
    native_name              = $3,
    is_active                = $4,
    is_rtl                   = $5,
    sort_order               = $6,
    date_format              = $7,
    time_format              = $8,
    decimal_separator        = $9,
    thousands_separator      = $10,
    first_day_of_week        = $11,
    -- NULLIF: a locale with no default country round-trips as '' through the merge,
    -- and '' violates the countries FK - blank means NULL here
    default_country_code     = NULLIF(UPPER($12), ''),
    flag_emoji               = $13,
    hreflang_value           = LOWER($14),
    script                   = $15,
    plural_rule              = $16,
    email_from_name_override = $17,
    description              = $18,
    metadata                 = $19,
    updated_at               = NOW()
WHERE code = LOWER($1)
RETURNING *;

-- name: DeleteLocale :exec
DELETE FROM locales WHERE code = LOWER($1);

-- name: CountLocaleReferences :one
-- Total references across every locale-bearing column - used by the delete guard to refuse with a friendly count instead of a raw FK violation
SELECT (
    (SELECT COUNT(*) FROM channels                WHERE default_locale = LOWER($1)) +
    (SELECT COUNT(*) FROM channel_locales         WHERE locale_code    = LOWER($1)) +
    (SELECT COUNT(*) FROM catalog_product_content    WHERE locale         = LOWER($1)) +
    (SELECT COUNT(*) FROM admin_role_permission_scopes WHERE locale         = LOWER($1)) +
    (SELECT COUNT(*) FROM config_values              WHERE locale         = LOWER($1))
)::bigint AS total;
