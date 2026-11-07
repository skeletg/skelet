-- One assistant turn or one agent call, counted once however often it's sent - the seen key gates every count in the same
-- statement, and Postgres runs the writing CTEs whether or not the select reads them
-- name: RecordAIActivity :one
WITH fresh AS (
    INSERT INTO ai_activity_seen (dedupe_key) VALUES (sqlc.arg(dedupe_key)) ON CONFLICT DO NOTHING RETURNING 1
), tools AS (
    INSERT INTO ai_tool_daily AS d (channel_id, day, source, agent, tool, calls)
    SELECT sqlc.arg(channel_id)::uuid, sqlc.arg(day)::date, sqlc.arg(source)::text, sqlc.arg(agent)::text, t.tool, count(*)
    FROM unnest(sqlc.arg(tools)::text[]) AS t(tool)
    WHERE EXISTS (SELECT 1 FROM fresh)
    GROUP BY t.tool
    ON CONFLICT (channel_id, day, source, agent, tool) DO UPDATE SET calls = d.calls + EXCLUDED.calls
    RETURNING 1
), searched AS (
    INSERT INTO ai_search_daily AS d (channel_id, day, source, agent, term, searches, empty)
    SELECT sqlc.arg(channel_id)::uuid, sqlc.arg(day)::date, sqlc.arg(source)::text, sqlc.arg(agent)::text, s.term, count(*),
        count(*) FILTER (WHERE (sqlc.arg(results)::int[])[s.i] = 0)
    FROM unnest(sqlc.arg(terms)::text[]) WITH ORDINALITY AS s(term, i)
    WHERE EXISTS (SELECT 1 FROM fresh)
    GROUP BY s.term
    ON CONFLICT (channel_id, day, source, agent, term) DO UPDATE SET searches = d.searches + EXCLUDED.searches, empty = d.empty + EXCLUDED.empty
    RETURNING 1
)
SELECT count(*)::bigint AS recorded FROM fresh;

-- What the assistant and the agents searched for and got nothing back, the busiest first
-- name: AISearchGaps :many
SELECT term,
    COALESCE(SUM(empty) FILTER (WHERE source = sqlc.arg(assistant_source)::text), 0)::bigint AS assistant,
    COALESCE(SUM(empty) FILTER (WHERE source = sqlc.arg(agent_source)::text), 0)::bigint AS agents
FROM ai_search_daily
WHERE channel_id = sqlc.arg(channel_id) AND day >= sqlc.arg(since)::date AND empty > 0
GROUP BY term
ORDER BY SUM(empty) DESC, term
LIMIT sqlc.arg(lim);

-- name: AIAgentTotals :many
SELECT agent, SUM(calls)::bigint AS calls
FROM ai_tool_daily
WHERE channel_id = sqlc.arg(channel_id) AND day >= sqlc.arg(since)::date AND source = sqlc.arg(source)::text
GROUP BY agent
ORDER BY calls DESC, agent
LIMIT sqlc.arg(lim);

-- name: AIAgentTools :many
SELECT agent, tool, SUM(calls)::bigint AS calls
FROM ai_tool_daily
WHERE channel_id = sqlc.arg(channel_id) AND day >= sqlc.arg(since)::date AND source = sqlc.arg(source)::text AND agent = ANY(sqlc.arg(agents)::text[])
GROUP BY agent, tool
ORDER BY agent, calls DESC, tool;

-- name: AIAgentSearches :many
SELECT agent, SUM(searches)::bigint AS searches, SUM(empty)::bigint AS empty
FROM ai_search_daily
WHERE channel_id = sqlc.arg(channel_id) AND day >= sqlc.arg(since)::date AND source = sqlc.arg(source)::text AND agent = ANY(sqlc.arg(agents)::text[])
GROUP BY agent;

-- name: PruneAIToolDaily :execrows
DELETE FROM ai_tool_daily WHERE day < sqlc.arg(before)::date;

-- name: PruneAISearchDaily :execrows
DELETE FROM ai_search_daily WHERE day < sqlc.arg(before)::date;

-- name: PruneAIActivitySeen :execrows
DELETE FROM ai_activity_seen WHERE seen_at < sqlc.arg(before);

-- name: PruneOrphanedAITools :execrows
DELETE FROM ai_tool_daily t WHERE NOT EXISTS (SELECT 1 FROM channel_roster c WHERE c.channel_id = t.channel_id);

-- name: PruneOrphanedAISearches :execrows
DELETE FROM ai_search_daily t WHERE NOT EXISTS (SELECT 1 FROM channel_roster c WHERE c.channel_id = t.channel_id);
