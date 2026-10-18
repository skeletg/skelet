-- name: CustomerRosterForSync :many
-- The roster the Signals database needs: identity for trait and membership cleanup, plus what a marketing send has to know to address someone - the suite reads only what core pushes from here
-- Both consent joins resolve to the registry default when no row exists, which for marketing is opted out
-- Keyset-paged so a delete mid-sweep can't shift a customer off a page; read_at stops an older push overwriting a newer one
-- Lifetime order totals ride along, since the suite's event log only keeps a year
SELECT c.id AS customer_id, c.channel_id, c.email, c.first_name,
       COALESCE(c.phone, '')::text AS phone, c.is_active, c.customer_group_id,
       COALESCE(g.slug, '')::text AS customer_group_slug,
       COALESCE(k.opted_in, false)::boolean AS marketing_opted_in,
       COALESCE(s.opted_in, false)::boolean AS sms_opted_in,
       COALESCE((SELECT jsonb_object_agg(cc.kind, cc.opted_in) FROM customer_consents cc WHERE cc.customer_id = c.id), '{}'::jsonb) AS consents,
       statement_timestamp()::timestamptz AS read_at,
       oa.order_count::int AS order_count, oa.monetary_cents::bigint AS monetary_cents,
       oa.first_order_at, oa.last_order_at
FROM customers c
LEFT JOIN customer_groups g ON g.id = c.customer_group_id
LEFT JOIN customer_consents k ON k.customer_id = c.id AND k.kind = 'marketing_email'
LEFT JOIN customer_consents s ON s.customer_id = c.id AND s.kind = 'marketing_sms'
CROSS JOIN LATERAL (
    SELECT count(*) AS order_count, COALESCE(sum(o.total_amount), 0) AS monetary_cents,
           min(o.created_at) AS first_order_at, max(o.created_at) AS last_order_at
    FROM orders o WHERE o.customer_id = c.id AND o.status NOT IN ('draft', 'cancelled')
) oa
WHERE c.id > sqlc.arg(after)::uuid
ORDER BY c.id
LIMIT sqlc.arg(lim);

-- name: CustomerRosterEntry :one
-- One customer for the event-driven push, shaped exactly like a page row
SELECT c.id AS customer_id, c.channel_id, c.email, c.first_name,
       COALESCE(c.phone, '')::text AS phone, c.is_active, c.customer_group_id,
       COALESCE(g.slug, '')::text AS customer_group_slug,
       COALESCE(k.opted_in, false)::boolean AS marketing_opted_in,
       COALESCE(s.opted_in, false)::boolean AS sms_opted_in,
       COALESCE((SELECT jsonb_object_agg(cc.kind, cc.opted_in) FROM customer_consents cc WHERE cc.customer_id = c.id), '{}'::jsonb) AS consents,
       statement_timestamp()::timestamptz AS read_at,
       oa.order_count::int AS order_count, oa.monetary_cents::bigint AS monetary_cents,
       oa.first_order_at, oa.last_order_at
FROM customers c
LEFT JOIN customer_groups g ON g.id = c.customer_group_id
LEFT JOIN customer_consents k ON k.customer_id = c.id AND k.kind = 'marketing_email'
LEFT JOIN customer_consents s ON s.customer_id = c.id AND s.kind = 'marketing_sms'
CROSS JOIN LATERAL (
    SELECT count(*) AS order_count, COALESCE(sum(o.total_amount), 0) AS monetary_cents,
           min(o.created_at) AS first_order_at, max(o.created_at) AS last_order_at
    FROM orders o WHERE o.customer_id = c.id AND o.status NOT IN ('draft', 'cancelled')
) oa
WHERE c.id = sqlc.arg(customer_id);
