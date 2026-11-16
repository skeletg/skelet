-- migrate:up
-- Scope is '' for the whole store or a channel id, period '' or the year - seeded so numbers continue where they were
CREATE TABLE order_number_counters (
    scope text NOT NULL,
    period text NOT NULL,
    next_value bigint NOT NULL,
    PRIMARY KEY (scope, period)
);
INSERT INTO order_number_counters (scope, period, next_value)
SELECT '', '', CASE WHEN is_called THEN last_value + 1 ELSE last_value END FROM order_number_seq;
CREATE FUNCTION next_order_number() RETURNS text LANGUAGE sql AS $$
    INSERT INTO order_number_counters (scope, period, next_value) VALUES ('', '', 2)
    ON CONFLICT (scope, period) DO UPDATE SET next_value = order_number_counters.next_value + 1
    RETURNING 'ORD-' || lpad((next_value - 1)::text, 6, '0')
$$;
ALTER TABLE orders ALTER COLUMN number SET DEFAULT next_order_number();
DROP SEQUENCE order_number_seq;

-- migrate:down
CREATE SEQUENCE order_number_seq;
SELECT setval('order_number_seq', next_value, false) FROM order_number_counters WHERE scope = '' AND period = '';
ALTER TABLE orders ALTER COLUMN number SET DEFAULT ('ORD-' || lpad(nextval('order_number_seq')::text, 6, '0'));
DROP FUNCTION next_order_number();
DROP TABLE order_number_counters;
