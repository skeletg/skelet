-- Synapse spine aggregates (moved from the commerce db package at cutover 4b).
-- Every query here reads analytics_events and/or analytics_identities - both now
-- live in the signals db, so the identity-resolution joins stay in-DB. The
-- commerce-coupled Synapse queries (segment membership / traits / orders /
-- email_sends) stay in the commerce db package; SegmentTopAffinities +
-- MostRecentEmailEngagement are rewritten app-side (they join commerce tables).
--
-- Every spine read excludes bot traffic (`is_bot` stamped at ingest) - Lens
-- already excludes bots from its rollups, and the two products must agree.

-- name: SynapseTotals :one
SELECT
    COUNT(*)                                          AS total_events,
    COUNT(*) FILTER (WHERE event_name = 'page_view')  AS page_views,
    COUNT(*) FILTER (WHERE event_name = 'conversion') AS conversions
FROM analytics_events
WHERE channel_id = sqlc.arg(channel_id) AND occurred_at >= sqlc.arg(since)
  AND NOT COALESCE((properties->>'is_bot')::boolean, FALSE);

-- name: SynapseIdentifiedCount :one
-- Distinct customers Synapse has stitched an identity for (this channel).
SELECT COUNT(DISTINCT customer_id) AS identified
FROM analytics_identities
WHERE channel_id = sqlc.arg(channel_id);

-- name: SynapseEventCounts :many
SELECT event_name, COUNT(*) AS count
FROM analytics_events
WHERE channel_id = sqlc.arg(channel_id) AND occurred_at >= sqlc.arg(since)
  AND NOT COALESCE((properties->>'is_bot')::boolean, FALSE)
GROUP BY event_name
ORDER BY count DESC;

-- name: SynapseActiveCustomers :many
-- Most-active customers by RESOLVED activity: events recorded directly against a
-- customer_id PLUS web events under any anon_id stitched to that customer.
SELECT
    COALESCE(e.customer_id, i.customer_id)::uuid AS customer_id,
    COUNT(*)                                     AS events,
    MAX(e.occurred_at)::timestamptz              AS last_seen
FROM analytics_events e
LEFT JOIN analytics_identities i ON i.anon_id = e.anon_id
WHERE e.channel_id = sqlc.arg(channel_id)
  AND e.occurred_at >= sqlc.arg(since)
  AND NOT COALESCE((e.properties->>'is_bot')::boolean, FALSE)
  AND COALESCE(e.customer_id, i.customer_id) IS NOT NULL
GROUP BY 1
ORDER BY events DESC
LIMIT sqlc.arg(lim);

-- name: FunnelEvents :many
-- Every funnel-step event for a channel since the cutoff, resolved to a person
-- (customer when known - directly or via a stitched anon - else the anon id) and
-- ordered (actor, time) so the read layer can greedily walk each actor through
-- the ordered steps. Only the step event names are pulled, and only for actors
-- who performed the FIRST step at all - anyone else can't enter the funnel, so
-- streaming their rows into the walker is pure waste (a funnel starting at
-- page_view still degenerates to everyone, but typical funnels start narrower).
WITH entrants AS (
    SELECT DISTINCT COALESCE(e.customer_id::text, i.customer_id::text, e.anon_id) AS actor
    FROM analytics_events e
    LEFT JOIN analytics_identities i ON i.anon_id = e.anon_id
    WHERE e.channel_id = sqlc.arg(channel_id)
      AND e.occurred_at >= sqlc.arg(since)
      AND e.event_name = sqlc.arg(first_step)::text
      AND NOT COALESCE((e.properties->>'is_bot')::boolean, FALSE)
      AND COALESCE(e.customer_id::text, i.customer_id::text, e.anon_id) IS NOT NULL
)
SELECT
    COALESCE(e.customer_id::text, i.customer_id::text, e.anon_id) AS actor,
    e.event_name                                                 AS event_name,
    e.occurred_at                                                AS occurred_at
FROM analytics_events e
LEFT JOIN analytics_identities i ON i.anon_id = e.anon_id
WHERE e.channel_id = sqlc.arg(channel_id)
  AND e.occurred_at >= sqlc.arg(since)
  AND e.event_name = ANY(sqlc.arg(steps)::text[])
  AND NOT COALESCE((e.properties->>'is_bot')::boolean, FALSE)
  AND COALESCE(e.customer_id::text, i.customer_id::text, e.anon_id) IN (SELECT actor FROM entrants)
ORDER BY actor, e.occurred_at;

-- name: InsightsSeries :many
-- Ad-hoc breakdown: per time bucket (day/week) × breakdown-property value, the
-- total events + distinct people. Optional event filter (''=any). breakdown is a
-- jsonb property key (''=no breakdown → all rows group under '(none)'). The
-- ::text casts on the parameters avoid the jsonb-`->>` / date_trunc typing traps.
SELECT
    date_trunc(sqlc.arg(unit)::text, e.occurred_at)::timestamptz                        AS bucket,
    COALESCE(e.properties->>sqlc.arg(prop_key)::text, '(none)')                          AS breakdown,
    COUNT(*)::bigint                                                                     AS total,
    COUNT(DISTINCT COALESCE(e.customer_id::text, i.customer_id::text, e.anon_id))::bigint AS uniques
FROM analytics_events e
LEFT JOIN analytics_identities i ON i.anon_id = e.anon_id
WHERE e.channel_id = sqlc.arg(channel_id)
  AND e.occurred_at >= sqlc.arg(since)
  AND NOT COALESCE((e.properties->>'is_bot')::boolean, FALSE)
  AND (sqlc.arg(event_name)::text = '' OR e.event_name = sqlc.arg(event_name)::text)
GROUP BY 1, 2
ORDER BY 1, 2;

-- name: EventWindowMeasure :one
-- Total events + distinct people for an event over a single window [start, end).
-- Called for the current and prior windows to drive Insights' period-over-period
-- delta. Optional event filter ('' = any). Resolved across anon + identity.
SELECT
    COUNT(*)::bigint                                                                     AS total,
    COUNT(DISTINCT COALESCE(e.customer_id::text, i.customer_id::text, e.anon_id))::bigint AS uniques
FROM analytics_events e
LEFT JOIN analytics_identities i ON i.anon_id = e.anon_id
WHERE e.channel_id = sqlc.arg(channel_id)
  AND e.occurred_at >= sqlc.arg(start_at) AND e.occurred_at < sqlc.arg(end_at)
  AND NOT COALESCE((e.properties->>'is_bot')::boolean, FALSE)
  AND (sqlc.arg(event_name)::text = '' OR e.event_name = sqlc.arg(event_name)::text);

-- name: ActiveByDay :many
-- Distinct active people per calendar day (resolved across anon + identity).
-- Optional event filter: pass '' to count any activity. Powers the DAU series.
SELECT
    date_trunc('day', e.occurred_at)::timestamptz                                       AS day,
    COUNT(DISTINCT COALESCE(e.customer_id::text, i.customer_id::text, e.anon_id))::bigint AS count
FROM analytics_events e
LEFT JOIN analytics_identities i ON i.anon_id = e.anon_id
WHERE e.channel_id = sqlc.arg(channel_id)
  AND e.occurred_at >= sqlc.arg(since)
  AND NOT COALESCE((e.properties->>'is_bot')::boolean, FALSE)
  AND (sqlc.arg(event_name)::text = '' OR e.event_name = sqlc.arg(event_name)::text)
  AND COALESCE(e.customer_id::text, i.customer_id::text, e.anon_id) IS NOT NULL
GROUP BY 1
ORDER BY 1;

-- name: ActiveDistinct :one
-- Distinct active people since the cutoff (resolved). Used for WAU (7d) / MAU
-- (30d). Optional event filter ('' = any activity).
SELECT COUNT(DISTINCT COALESCE(e.customer_id::text, i.customer_id::text, e.anon_id))::bigint AS count
FROM analytics_events e
LEFT JOIN analytics_identities i ON i.anon_id = e.anon_id
WHERE e.channel_id = sqlc.arg(channel_id)
  AND e.occurred_at >= sqlc.arg(since)
  AND NOT COALESCE((e.properties->>'is_bot')::boolean, FALSE)
  AND (sqlc.arg(event_name)::text = '' OR e.event_name = sqlc.arg(event_name)::text)
  AND COALESCE(e.customer_id::text, i.customer_id::text, e.anon_id) IS NOT NULL;

-- name: ReturnWindowStats :one
-- The commerce-friendly engagement metric: of the distinct people active in the
-- PREVIOUS window [prev_start, cur_start), how many were also active in the
-- CURRENT window [cur_start, cur_end) - i.e. came back. Returns prev-active,
-- cur-active and the returning overlap; the read layer derives the return rate +
-- new-vs-returning split. Active = any tracked event (optional event filter),
-- resolved across anon + identity. Window length is the store's return cadence.
WITH prev AS (
    SELECT DISTINCT COALESCE(e.customer_id::text, i.customer_id::text, e.anon_id) AS actor
    FROM analytics_events e
    LEFT JOIN analytics_identities i ON i.anon_id = e.anon_id
    WHERE e.channel_id = sqlc.arg(channel_id)
      AND e.occurred_at >= sqlc.arg(prev_start) AND e.occurred_at < sqlc.arg(cur_start)
      AND NOT COALESCE((e.properties->>'is_bot')::boolean, FALSE)
      AND (sqlc.arg(event_name)::text = '' OR e.event_name = sqlc.arg(event_name)::text)
      AND COALESCE(e.customer_id::text, i.customer_id::text, e.anon_id) IS NOT NULL
),
cur AS (
    SELECT DISTINCT COALESCE(e.customer_id::text, i.customer_id::text, e.anon_id) AS actor
    FROM analytics_events e
    LEFT JOIN analytics_identities i ON i.anon_id = e.anon_id
    WHERE e.channel_id = sqlc.arg(channel_id)
      AND e.occurred_at >= sqlc.arg(cur_start) AND e.occurred_at < sqlc.arg(cur_end)
      AND NOT COALESCE((e.properties->>'is_bot')::boolean, FALSE)
      AND (sqlc.arg(event_name)::text = '' OR e.event_name = sqlc.arg(event_name)::text)
      AND COALESCE(e.customer_id::text, i.customer_id::text, e.anon_id) IS NOT NULL
)
SELECT
    (SELECT COUNT(*) FROM prev)::bigint                                      AS prior_active,
    (SELECT COUNT(*) FROM cur)::bigint                                       AS active_now,
    (SELECT COUNT(*) FROM cur WHERE actor IN (SELECT actor FROM prev))::bigint AS returning;

-- name: FlowEvents :many
-- Every event of every FOCUS-EVENT actor for a channel since the cutoff,
-- resolved to a person and ordered (actor, time) - the read layer walks each
-- actor's sequence to build the flow path tree. Unfiltered by event within an
-- actor (a flow shows whatever people do next), but actors who never did the
-- focus event contribute nothing to the tree, so pulling their rows was pure
-- waste (the unbounded form streams the channel's ENTIRE event history)
-- Custom (Lens track()) events surface by their real name (properties->>'name')
-- instead of collapsing into one opaque "custom" step - the focus matcher below
-- mirrors that so a custom event works as the focus; the $experiment exposure
-- ping is dropped (Prism instrumentation, not a user action).
WITH focus_actors AS (
    SELECT DISTINCT COALESCE(e.customer_id::text, i.customer_id::text, e.anon_id) AS actor
    FROM analytics_events e
    LEFT JOIN analytics_identities i ON i.anon_id = e.anon_id
    WHERE e.channel_id = sqlc.arg(channel_id)
      AND e.occurred_at >= sqlc.arg(since)
      AND (e.event_name = sqlc.arg(focus)::text
           OR (e.event_name = 'custom' AND e.properties->>'name' = sqlc.arg(focus)::text))
      AND NOT COALESCE((e.properties->>'is_bot')::boolean, FALSE)
      AND COALESCE(e.customer_id::text, i.customer_id::text, e.anon_id) IS NOT NULL
)
SELECT
    COALESCE(e.customer_id::text, i.customer_id::text, e.anon_id) AS actor,
    CASE WHEN e.event_name = 'custom'
         THEN COALESCE(NULLIF(e.properties->>'name', ''), 'custom')
         ELSE e.event_name END                                   AS event_name,
    e.occurred_at                                                AS occurred_at
FROM analytics_events e
LEFT JOIN analytics_identities i ON i.anon_id = e.anon_id
WHERE e.channel_id = sqlc.arg(channel_id)
  AND e.occurred_at >= sqlc.arg(since)
  AND NOT COALESCE((e.properties->>'is_bot')::boolean, FALSE)
  AND COALESCE(e.customer_id::text, i.customer_id::text, e.anon_id) IN (SELECT actor FROM focus_actors)
  AND NOT (e.event_name = 'custom' AND e.properties->>'name' = '$experiment')
ORDER BY actor, e.occurred_at;

-- name: SegmentEventActors :many
-- Customers (resolved directly or via a stitched anon) who did a given event at
-- least min_count times since the cutoff - one building block of a behavioral
-- segment. Anonymous-only people are excluded (segments hold customer ids).
SELECT COALESCE(e.customer_id, i.customer_id)::uuid AS customer_id
FROM analytics_events e
LEFT JOIN analytics_identities i ON i.anon_id = e.anon_id
WHERE e.channel_id = sqlc.arg(channel_id)
  AND e.event_name = sqlc.arg(event_name)
  AND e.occurred_at >= sqlc.arg(since)
  AND NOT COALESCE((e.properties->>'is_bot')::boolean, FALSE)
  AND COALESCE(e.customer_id, i.customer_id) IS NOT NULL
GROUP BY 1
HAVING COUNT(*) >= sqlc.arg(min_count)::bigint;

-- name: AffinityActors :many
-- Customers (resolved directly or via a stitched anon) with >=1 event carrying
-- the affinity prop (attr:<slug>) = value since the cutoff. Powers affinity
-- segments (e.g. "tire size = 205/55R16").
SELECT COALESCE(e.customer_id, i.customer_id)::uuid AS customer_id
FROM analytics_events e
LEFT JOIN analytics_identities i ON i.anon_id = e.anon_id
WHERE e.channel_id = sqlc.arg(channel_id)
  AND e.occurred_at >= sqlc.arg(since)
  AND NOT COALESCE((e.properties->>'is_bot')::boolean, FALSE)
  AND e.properties->>sqlc.arg(prop_key)::text = sqlc.arg(value)::text
  AND COALESCE(e.customer_id, i.customer_id) IS NOT NULL
GROUP BY 1;

-- name: AffinityValueActorCounts :many
-- Distinct values of one affinity prop (attr:<slug>) for a channel since the
-- cutoff, with the number of DISTINCT identified customers carrying each value.
-- Powers auto-generated affinity segments: only values shared by enough
-- customers are worth a segment. Anonymous-only people are excluded (segments
-- hold customer ids). Ordered by reach so callers can take the top N.
SELECT
    COALESCE(e.properties->>sqlc.arg(prop_key)::text, '')::text       AS value,
    COUNT(DISTINCT COALESCE(e.customer_id, i.customer_id))::bigint    AS actors
FROM analytics_events e
LEFT JOIN analytics_identities i ON i.anon_id = e.anon_id
WHERE e.channel_id = sqlc.arg(channel_id)
  AND e.occurred_at >= sqlc.arg(since)
  AND NOT COALESCE((e.properties->>'is_bot')::boolean, FALSE)
  AND e.properties->>sqlc.arg(prop_key)::text IS NOT NULL
  AND e.properties->>sqlc.arg(prop_key)::text <> ''
  AND COALESCE(e.customer_id, i.customer_id) IS NOT NULL
GROUP BY 1
ORDER BY actors DESC, value
LIMIT sqlc.arg(lim);

-- name: SynapseAnonCount :one
-- Distinct ANONYMOUS visitors (this channel, window): anon ids with activity
-- that have NOT been stitched to a customer yet. Once a visitor identifies,
-- their activity rolls into the customer profile, so they drop off this count.
SELECT COUNT(DISTINCT e.anon_id)::bigint AS anon
FROM analytics_events e
WHERE e.channel_id = sqlc.arg(channel_id)
  AND e.occurred_at >= sqlc.arg(since)
  AND NOT COALESCE((e.properties->>'is_bot')::boolean, FALSE)
  AND e.anon_id IS NOT NULL AND e.anon_id <> ''
  AND NOT EXISTS (SELECT 1 FROM analytics_identities i WHERE i.anon_id = e.anon_id);

-- name: SynapseActiveVisitors :many
-- Most-active ANONYMOUS visitors (not yet identified) by event volume, with the
-- most recent page title/key seen so the list reads as more than a raw id.
SELECT
    e.anon_id                                   AS anon_id,
    COUNT(*)                                    AS events,
    MAX(e.occurred_at)::timestamptz             AS last_seen,
    (SELECT COALESCE(le.properties->>'title', le.properties->>'page_key', '')
       FROM analytics_events le
      WHERE le.anon_id = e.anon_id AND le.channel_id = sqlc.arg(channel_id)
        AND le.event_name = 'page_view'
      ORDER BY le.occurred_at DESC
      LIMIT 1)                                  AS last_page
FROM analytics_events e
WHERE e.channel_id = sqlc.arg(channel_id)
  AND e.occurred_at >= sqlc.arg(since)
  AND NOT COALESCE((e.properties->>'is_bot')::boolean, FALSE)
  AND e.anon_id IS NOT NULL AND e.anon_id <> ''
  AND NOT EXISTS (SELECT 1 FROM analytics_identities i WHERE i.anon_id = e.anon_id)
GROUP BY e.anon_id
ORDER BY events DESC
LIMIT sqlc.arg(lim);

-- name: CustomerExperimentArms :many
-- The experiments + assigned variation a customer has been exposed to, resolved
-- through identity stitching (their anon ids' $experiment exposures). Makes the
-- A/B arm a visible Synapse trait, targetable by Pulse. Pure spine read - the
-- caller resolves the customer's anon ids first (AnonIDsForCustomer).
SELECT ev.properties->>'experiment' AS experiment, COALESCE(ev.properties->>'variation', '') AS variation, MAX(ev.occurred_at) AS last_exposed_at
FROM analytics_events ev
WHERE ev.source = 'web' AND ev.event_name = 'custom'
  AND ev.properties->>'name' = '$experiment'
  AND ev.properties->>'experiment' IS NOT NULL
  AND ev.occurred_at >= sqlc.arg(since)
  AND ev.anon_id = ANY(sqlc.arg(anon_ids)::text[])
GROUP BY 1, 2
ORDER BY last_exposed_at DESC;

-- name: SegmentAffinitiesForMembers :many
-- Spine half of the app-side SegmentTopAffinities read: top attr:<slug> = value
-- among a segment's members (their customer ids passed in - the commerce half,
-- customer_segment_memberships, lives in the commerce db). Distinct-member counts,
-- resolved across customer_id + stitched anon ids. Channel- and time-bounded (the
-- CROSS JOIN LATERAL jsonb-expands every row it touches).
SELECT kv.k AS attr_key, kv.v AS attr_val, COUNT(DISTINCT COALESCE(e.customer_id, i.customer_id))::int AS members
FROM analytics_events e
LEFT JOIN analytics_identities i ON i.anon_id = e.anon_id
CROSS JOIN LATERAL jsonb_each_text(CASE WHEN jsonb_typeof(e.properties) = 'object' THEN e.properties ELSE '{}'::jsonb END) AS kv(k, v)
WHERE e.channel_id = sqlc.arg(channel_id)
  AND e.occurred_at >= sqlc.arg(since)
  AND NOT COALESCE((e.properties->>'is_bot')::boolean, FALSE)
  AND COALESCE(e.customer_id, i.customer_id) = ANY(sqlc.arg(member_ids)::uuid[])
  AND kv.k LIKE 'attr:%' AND kv.v <> ''
GROUP BY kv.k, kv.v
ORDER BY members DESC, kv.k
LIMIT sqlc.arg(lim);

-- name: SegmentAffinitiesForMembersSmall :many
-- Index-friendly variant for ordinary segment sizes: the OR arms hit the partial
-- customer/anon time indexes (BitmapOr) where the bare COALESCE above can only
-- scan the channel window. The COALESCE condition stays as the exact filter -
-- the anon arm alone would admit events whose customer_id is a non-member. The
-- caller resolves the members' anon ids first and picks this variant when the
-- arrays are small enough to probe.
SELECT kv.k AS attr_key, kv.v AS attr_val, COUNT(DISTINCT COALESCE(e.customer_id, i.customer_id))::int AS members
FROM analytics_events e
LEFT JOIN analytics_identities i ON i.anon_id = e.anon_id
CROSS JOIN LATERAL jsonb_each_text(CASE WHEN jsonb_typeof(e.properties) = 'object' THEN e.properties ELSE '{}'::jsonb END) AS kv(k, v)
WHERE e.channel_id = sqlc.arg(channel_id)
  AND e.occurred_at >= sqlc.arg(since)
  AND NOT COALESCE((e.properties->>'is_bot')::boolean, FALSE)
  AND (e.customer_id = ANY(sqlc.arg(member_ids)::uuid[]) OR e.anon_id = ANY(sqlc.arg(member_anon_ids)::text[]))
  AND COALESCE(e.customer_id, i.customer_id) = ANY(sqlc.arg(member_ids)::uuid[])
  AND kv.k LIKE 'attr:%' AND kv.v <> ''
GROUP BY kv.k, kv.v
ORDER BY members DESC, kv.k
LIMIT sqlc.arg(lim);

-- name: CustomerEngagementEvents :many
-- Spine half of the app-side conversion-attribution lookback (recorder's
-- RecordConversion): every marketing open/click the customer logged since the
-- cutoff, best-first (a click outranks an open; then most-recent). The caller
-- resolves each send's origin from email_sends (commerce) and takes the first
-- from a campaign or flow - the last-touch anchor. The per-customer 7-day set is
-- small, so this is unbounded (the winner is picked app-side, not via LIMIT 1).
SELECT event_name, occurred_at, send_id
FROM analytics_events
WHERE customer_id = sqlc.arg(customer_id)
  AND event_name IN ('email_opened', 'email_clicked')
  AND send_id IS NOT NULL
  AND occurred_at >= sqlc.arg(since)
ORDER BY (event_name = 'email_clicked') DESC, occurred_at DESC;
