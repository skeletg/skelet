-- Prism: the experiments table plus the spine reads that score it. Exposures and
-- goals ride analytics_events and resolve through analytics_identities, so a
-- results query reads the definition and its numbers from the one database.

-- name: ExperimentGoalEvents :many
-- Pickable goal-event suggestions for this channel, most-frequent first: web
-- custom-event names (props.name, excluding the internal '$experiment') PLUS
-- commerce/account spine event names (e.g. order_placed) so experiments can
-- measure real conversions, not just clicks.
SELECT name, SUM(uses)::int AS uses
FROM (
    SELECT ev.properties->>'name' AS name, COUNT(*) AS uses
    FROM analytics_events ev
    WHERE ev.source = 'web' AND ev.event_name = 'custom'
      AND ev.channel_id = sqlc.arg(channel_id)
      AND ev.occurred_at >= sqlc.arg(since)
      AND ev.properties->>'name' IS NOT NULL
      AND ev.properties->>'name' <> '$experiment'
    GROUP BY 1
    UNION ALL
    SELECT ev.event_name AS name, COUNT(*) AS uses
    FROM analytics_events ev
    WHERE ev.source IN ('commerce', 'account')
      AND ev.channel_id = sqlc.arg(channel_id)
      AND ev.occurred_at >= sqlc.arg(since)
    GROUP BY 1
) g
GROUP BY name
ORDER BY uses DESC, name ASC
LIMIT 100;

-- name: ExperimentResults :many
-- Per-variation exposed / converted / revenue. Exposures ride the Lens custom-
-- event pipeline (props.name='$experiment', anon-keyed). The goal can be either
-- a web custom event (matched by props.name) OR any spine event whose event_name
-- IS the goal (e.g. the commerce 'order_placed') - so A/B tests can measure real
-- orders + revenue, not just clicks. Identity stitching bridges the anon-keyed
-- exposure to a customer-keyed commerce goal (goal.anon_id = exposure anon, OR
-- goal.customer_id = the customer that anon maps to). One conversion per person;
-- revenue sums revenue_cents/total_cents of the matched goal events.
WITH exposures AS (
    SELECT COALESCE(ev.properties->>'variation', '') AS variation, ev.anon_id AS anon_id, MIN(ev.occurred_at) AS first_exp
    FROM analytics_events ev
    WHERE ev.source = 'web' AND ev.event_name = 'custom'
      AND ev.properties->>'name' = '$experiment'
      AND ev.properties->>'experiment' = sqlc.arg(key)::text
      AND ev.channel_id = sqlc.arg(channel_id)
      AND ev.occurred_at >= sqlc.arg(since)
      AND ev.anon_id IS NOT NULL
    GROUP BY 1, ev.anon_id
),
exp_ident AS (
    SELECT e.variation, e.anon_id, e.first_exp, ai.customer_id
    FROM exposures e
    LEFT JOIN analytics_identities ai ON ai.anon_id = e.anon_id
),
goals AS (
    SELECT ev.anon_id AS anon_id, ev.customer_id AS customer_id, ev.occurred_at AS occurred_at,
           COALESCE(NULLIF(ev.properties->>'revenue_cents', '')::bigint, NULLIF(ev.properties->>'total_cents', '')::bigint, 0) AS revenue_cents
    FROM analytics_events ev
    WHERE ev.channel_id = sqlc.arg(channel_id)
      AND ev.occurred_at >= sqlc.arg(since)
      AND ((ev.event_name = 'custom' AND ev.properties->>'name' = sqlc.arg(goal_event)::text)
           OR ev.event_name = sqlc.arg(goal_event)::text)
)
SELECT
    x.variation                                              AS variation,
    COUNT(*)::int                                            AS exposed,
    COUNT(*) FILTER (WHERE conv.matched)::int                AS converted,
    COALESCE(SUM(conv.revenue) FILTER (WHERE conv.matched), 0)::bigint AS revenue_cents
FROM exp_ident x
LEFT JOIN LATERAL (
    SELECT COUNT(*) > 0 AS matched, COALESCE(SUM(g.revenue_cents), 0) AS revenue
    FROM goals g
    WHERE g.occurred_at >= x.first_exp
      AND (g.anon_id = x.anon_id OR (x.customer_id IS NOT NULL AND g.customer_id = x.customer_id))
) conv ON TRUE
GROUP BY x.variation
ORDER BY x.variation;

-- name: CreateExperiment :one
INSERT INTO experiments (channel_id, key, name, status, coverage, goal_event, variations, segment_slug)
VALUES (sqlc.arg(channel_id), sqlc.arg(key), sqlc.arg(name), sqlc.arg(status), sqlc.arg(coverage), sqlc.arg(goal_event), sqlc.arg(variations), sqlc.arg(segment_slug))
RETURNING *;

-- name: UpdateExperiment :execrows
UPDATE experiments
SET name = sqlc.arg(name), status = sqlc.arg(status), coverage = sqlc.arg(coverage),
    goal_event = sqlc.arg(goal_event), variations = sqlc.arg(variations), segment_slug = sqlc.arg(segment_slug), updated_at = NOW()
WHERE id = sqlc.arg(id);

-- name: GetExperiment :one
SELECT * FROM experiments WHERE id = sqlc.arg(id);

-- name: ListExperiments :many
SELECT * FROM experiments WHERE channel_id = sqlc.arg(channel_id) ORDER BY created_at DESC;

-- name: ListRunningExperiments :many
SELECT * FROM experiments WHERE channel_id = sqlc.arg(channel_id) AND status = 'running' ORDER BY key;

-- name: DeleteExperiment :exec
DELETE FROM experiments WHERE id = sqlc.arg(id);
