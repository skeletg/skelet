-- migrate:up
-- One name per moment: Signals' own event names become GA4's where GA4 has one, the same names the tracking plugins
-- and any front-end tag use. Saved definitions name events, so they're rewritten with the spine (its own migration in
-- spinech/migrations): board reports and staged drafts, behavioral segments, and experiment goals. Only the values
-- under event, startEvent, returnEvent and steps change - a title or a filter value that happens to read like an
-- event name is left alone. Pulse triggers are their own vocabulary and keep theirs
CREATE OR REPLACE FUNCTION pg_temp.renamed_event_refs(doc jsonb, renames jsonb) RETURNS jsonb LANGUAGE plpgsql IMMUTABLE AS $$
BEGIN
    CASE jsonb_typeof(doc)
    WHEN 'object' THEN
        RETURN (SELECT COALESCE(jsonb_object_agg(k, CASE
            WHEN k IN ('event', 'startEvent', 'returnEvent') AND jsonb_typeof(v) = 'string'
                THEN COALESCE(renames -> (v #>> '{}'), v)
            WHEN k = 'steps' AND jsonb_typeof(v) = 'array'
                THEN (SELECT COALESCE(jsonb_agg(CASE WHEN jsonb_typeof(s) = 'string' THEN COALESCE(renames -> (s #>> '{}'), s) ELSE s END ORDER BY i), '[]'::jsonb)
                      FROM jsonb_array_elements(v) WITH ORDINALITY AS a(s, i))
            ELSE pg_temp.renamed_event_refs(v, renames)
        END), '{}'::jsonb) FROM jsonb_each(doc) AS e(k, v));
    WHEN 'array' THEN
        RETURN (SELECT COALESCE(jsonb_agg(pg_temp.renamed_event_refs(x, renames) ORDER BY i), '[]'::jsonb)
                FROM jsonb_array_elements(doc) WITH ORDINALITY AS a(x, i));
    ELSE
        RETURN doc;
    END CASE;
END $$;

CREATE TEMP TABLE event_renames ON COMMIT DROP AS SELECT '{
    "cart_add": "add_to_cart", "cart_remove": "remove_from_cart", "cart_update": "cart_updated",
    "checkout_shipping": "add_shipping_info", "checkout_payment": "add_payment_info",
    "wishlist_add": "add_to_wishlist", "wishlist_remove": "remove_from_wishlist",
    "order_placed": "purchase", "order_refunded": "refund", "registered": "sign_up", "lead": "generate_lead",
    "password_reset": "password_reset_requested", "$search": "search", "product_filter": "filter_items",
    "search_overview_click": "select_overview_item", "search_overview_chat": "open_overview_chat"
}'::jsonb AS m;

UPDATE board_reports SET definition = pg_temp.renamed_event_refs(definition, (SELECT m FROM event_renames));
UPDATE board_drafts SET payload = pg_temp.renamed_event_refs(payload, (SELECT m FROM event_renames));
UPDATE customer_segments SET rules = pg_temp.renamed_event_refs(rules, (SELECT m FROM event_renames)) WHERE rules IS NOT NULL;
UPDATE customer_segments SET definition = pg_temp.renamed_event_refs(definition::jsonb, (SELECT m FROM event_renames))::text
WHERE pg_input_is_valid(definition, 'jsonb') AND pg_temp.renamed_event_refs(definition::jsonb, (SELECT m FROM event_renames)) <> definition::jsonb;
UPDATE experiments SET goal_event = (SELECT m ->> goal_event FROM event_renames)
WHERE (SELECT m ? goal_event FROM event_renames);

-- migrate:down
CREATE OR REPLACE FUNCTION pg_temp.renamed_event_refs(doc jsonb, renames jsonb) RETURNS jsonb LANGUAGE plpgsql IMMUTABLE AS $$
BEGIN
    CASE jsonb_typeof(doc)
    WHEN 'object' THEN
        RETURN (SELECT COALESCE(jsonb_object_agg(k, CASE
            WHEN k IN ('event', 'startEvent', 'returnEvent') AND jsonb_typeof(v) = 'string'
                THEN COALESCE(renames -> (v #>> '{}'), v)
            WHEN k = 'steps' AND jsonb_typeof(v) = 'array'
                THEN (SELECT COALESCE(jsonb_agg(CASE WHEN jsonb_typeof(s) = 'string' THEN COALESCE(renames -> (s #>> '{}'), s) ELSE s END ORDER BY i), '[]'::jsonb)
                      FROM jsonb_array_elements(v) WITH ORDINALITY AS a(s, i))
            ELSE pg_temp.renamed_event_refs(v, renames)
        END), '{}'::jsonb) FROM jsonb_each(doc) AS e(k, v));
    WHEN 'array' THEN
        RETURN (SELECT COALESCE(jsonb_agg(pg_temp.renamed_event_refs(x, renames) ORDER BY i), '[]'::jsonb)
                FROM jsonb_array_elements(doc) WITH ORDINALITY AS a(x, i));
    ELSE
        RETURN doc;
    END CASE;
END $$;

CREATE TEMP TABLE event_renames ON COMMIT DROP AS SELECT '{
    "add_to_cart": "cart_add", "remove_from_cart": "cart_remove", "cart_updated": "cart_update",
    "add_shipping_info": "checkout_shipping", "add_payment_info": "checkout_payment",
    "add_to_wishlist": "wishlist_add", "remove_from_wishlist": "wishlist_remove",
    "purchase": "order_placed", "refund": "order_refunded", "sign_up": "registered", "generate_lead": "lead",
    "password_reset_requested": "password_reset", "search": "$search", "filter_items": "product_filter",
    "select_overview_item": "search_overview_click", "open_overview_chat": "search_overview_chat"
}'::jsonb AS m;

UPDATE board_reports SET definition = pg_temp.renamed_event_refs(definition, (SELECT m FROM event_renames));
UPDATE board_drafts SET payload = pg_temp.renamed_event_refs(payload, (SELECT m FROM event_renames));
UPDATE customer_segments SET rules = pg_temp.renamed_event_refs(rules, (SELECT m FROM event_renames)) WHERE rules IS NOT NULL;
UPDATE customer_segments SET definition = pg_temp.renamed_event_refs(definition::jsonb, (SELECT m FROM event_renames))::text
WHERE pg_input_is_valid(definition, 'jsonb') AND pg_temp.renamed_event_refs(definition::jsonb, (SELECT m FROM event_renames)) <> definition::jsonb;
UPDATE experiments SET goal_event = (SELECT m ->> goal_event FROM event_renames)
WHERE (SELECT m ? goal_event FROM event_renames);
