-- name: CustomerRosterForSync :many
-- The roster the Signals database needs: identity for trait and membership cleanup, plus what a marketing send has to know to address someone - the suite reads only what core pushes from here
-- Both consent joins resolve to the registry default when no row exists, which for marketing is opted out
SELECT c.id AS customer_id, c.channel_id, c.email, c.first_name,
       COALESCE(c.phone, '')::text AS phone, c.is_active, c.customer_group_id,
       COALESCE(g.slug, '')::text AS customer_group_slug,
       COALESCE(k.opted_in, false)::boolean AS marketing_opted_in,
       COALESCE(s.opted_in, false)::boolean AS sms_opted_in,
       COALESCE((SELECT jsonb_object_agg(cc.kind, cc.opted_in) FROM customer_consents cc WHERE cc.customer_id = c.id), '{}'::jsonb) AS consents
FROM customers c
LEFT JOIN customer_groups g ON g.id = c.customer_group_id
LEFT JOIN customer_consents k ON k.customer_id = c.id AND k.kind = 'marketing_email'
LEFT JOIN customer_consents s ON s.customer_id = c.id AND s.kind = 'marketing_sms'
ORDER BY c.id
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: CustomerRosterEntry :one
-- One customer for the event-driven push, shaped exactly like a page row
SELECT c.id AS customer_id, c.channel_id, c.email, c.first_name,
       COALESCE(c.phone, '')::text AS phone, c.is_active, c.customer_group_id,
       COALESCE(g.slug, '')::text AS customer_group_slug,
       COALESCE(k.opted_in, false)::boolean AS marketing_opted_in,
       COALESCE(s.opted_in, false)::boolean AS sms_opted_in,
       COALESCE((SELECT jsonb_object_agg(cc.kind, cc.opted_in) FROM customer_consents cc WHERE cc.customer_id = c.id), '{}'::jsonb) AS consents
FROM customers c
LEFT JOIN customer_groups g ON g.id = c.customer_group_id
LEFT JOIN customer_consents k ON k.customer_id = c.id AND k.kind = 'marketing_email'
LEFT JOIN customer_consents s ON s.customer_id = c.id AND s.kind = 'marketing_sms'
WHERE c.id = sqlc.arg(customer_id);
