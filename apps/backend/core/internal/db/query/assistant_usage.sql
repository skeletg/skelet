-- name: RecordAssistantTurn :exec
INSERT INTO assistant_usage_daily AS u (
    channel_id, day, turns, failed_turns, unanswered_turns, cart_turns, checkout_turns,
    input_tokens, output_tokens, cache_read_tokens, cache_write_tokens
) VALUES (
    sqlc.arg(channel_id), (now() AT TIME ZONE 'UTC')::date, 1, sqlc.arg(failed_turns), sqlc.arg(unanswered_turns), sqlc.arg(cart_turns), sqlc.arg(checkout_turns),
    sqlc.arg(input_tokens), sqlc.arg(output_tokens), sqlc.arg(cache_read_tokens), sqlc.arg(cache_write_tokens)
)
ON CONFLICT (channel_id, day) DO UPDATE SET
    turns              = u.turns + 1,
    failed_turns       = u.failed_turns + EXCLUDED.failed_turns,
    unanswered_turns   = u.unanswered_turns + EXCLUDED.unanswered_turns,
    cart_turns         = u.cart_turns + EXCLUDED.cart_turns,
    checkout_turns     = u.checkout_turns + EXCLUDED.checkout_turns,
    input_tokens       = u.input_tokens + EXCLUDED.input_tokens,
    output_tokens      = u.output_tokens + EXCLUDED.output_tokens,
    cache_read_tokens  = u.cache_read_tokens + EXCLUDED.cache_read_tokens,
    cache_write_tokens = u.cache_write_tokens + EXCLUDED.cache_write_tokens;

-- A search overview spends tokens without being a chat turn
-- name: RecordAssistantOverview :exec
INSERT INTO assistant_usage_daily AS u (channel_id, day, overviews, input_tokens, output_tokens, cache_read_tokens, cache_write_tokens)
VALUES (sqlc.arg(channel_id), (now() AT TIME ZONE 'UTC')::date, sqlc.arg(overviews)::int, sqlc.arg(input_tokens), sqlc.arg(output_tokens), sqlc.arg(cache_read_tokens), sqlc.arg(cache_write_tokens))
ON CONFLICT (channel_id, day) DO UPDATE SET
    overviews          = u.overviews + EXCLUDED.overviews,
    input_tokens       = u.input_tokens + EXCLUDED.input_tokens,
    output_tokens      = u.output_tokens + EXCLUDED.output_tokens,
    cache_read_tokens  = u.cache_read_tokens + EXCLUDED.cache_read_tokens,
    cache_write_tokens = u.cache_write_tokens + EXCLUDED.cache_write_tokens;

-- name: AssistantUsageSince :one
SELECT
    COALESCE(SUM(turns), 0)::bigint              AS turns,
    COALESCE(SUM(overviews), 0)::bigint          AS overviews,
    COALESCE(SUM(failed_turns), 0)::bigint       AS failed_turns,
    COALESCE(SUM(unanswered_turns), 0)::bigint   AS unanswered_turns,
    COALESCE(SUM(cart_turns), 0)::bigint         AS cart_turns,
    COALESCE(SUM(checkout_turns), 0)::bigint     AS checkout_turns,
    COALESCE(SUM(input_tokens), 0)::bigint       AS input_tokens,
    COALESCE(SUM(output_tokens), 0)::bigint      AS output_tokens,
    COALESCE(SUM(cache_read_tokens), 0)::bigint  AS cache_read_tokens,
    COALESCE(SUM(cache_write_tokens), 0)::bigint AS cache_write_tokens
FROM assistant_usage_daily
WHERE channel_id = sqlc.arg(channel_id) AND day >= sqlc.arg(since)::date;

-- name: AssistedOrdersSince :many
SELECT
    currency_code::text                                                              AS currency_code,
    COUNT(*)::integer                                                                AS order_count,
    COUNT(*) FILTER (WHERE assisted_by = 'assistant')::integer                       AS assistant_orders,
    COALESCE(SUM(total_amount) FILTER (WHERE assisted_by = 'assistant'), 0)::bigint  AS assistant_revenue,
    COUNT(*) FILTER (WHERE assisted_by = 'agent')::integer                           AS agent_orders,
    COALESCE(SUM(total_amount) FILTER (WHERE assisted_by = 'agent'), 0)::bigint      AS agent_revenue
FROM orders
WHERE channel_id = sqlc.arg(channel_id) AND created_at >= sqlc.arg(since) AND status NOT IN ('cancelled', 'draft')
GROUP BY currency_code
ORDER BY order_count DESC;

-- name: AssistantUsageDaily :many
SELECT day, turns::bigint AS turns, failed_turns::bigint AS failed_turns, unanswered_turns::bigint AS unanswered_turns, cart_turns::bigint AS cart_turns,
    checkout_turns::bigint AS checkout_turns
FROM assistant_usage_daily
WHERE channel_id = sqlc.arg(channel_id) AND day >= sqlc.arg(since)::date
ORDER BY day;

-- name: AssistedOrdersDaily :many
SELECT
    (created_at AT TIME ZONE 'UTC')::date                         AS day,
    COUNT(*) FILTER (WHERE assisted_by = 'assistant')::integer    AS assistant_orders,
    COUNT(*) FILTER (WHERE assisted_by = 'agent')::integer        AS agent_orders
FROM orders
WHERE channel_id = sqlc.arg(channel_id) AND created_at >= sqlc.arg(since) AND status NOT IN ('cancelled', 'draft') AND assisted_by <> ''
GROUP BY 1
ORDER BY 1;

-- name: AssistantTokensToday :one
-- Cache reads cost a tenth and stay out of the count
SELECT COALESCE(SUM(input_tokens + output_tokens + cache_write_tokens), 0)::bigint AS tokens
FROM assistant_usage_daily
WHERE channel_id = sqlc.arg(channel_id) AND day = (now() AT TIME ZONE 'UTC')::date;
