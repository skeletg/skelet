-- migrate:up
-- The dashboard's ⌘K lookup matches orders on number, email and the name they ship to, and customers on email and full
-- name, as a substring - trigram indexes are what let each OR branch be an index scan instead of reading every row
CREATE INDEX idx_orders_number_trgm ON orders USING gin (number gin_trgm_ops);
CREATE INDEX idx_orders_email_trgm ON orders USING gin (email gin_trgm_ops);
CREATE INDEX idx_orders_shipping_name_trgm ON orders USING gin ((shipping_first_name || ' ' || shipping_last_name) gin_trgm_ops);
CREATE INDEX idx_customers_full_name_trgm ON customers USING gin ((first_name || ' ' || last_name) gin_trgm_ops);

-- migrate:down
DROP INDEX IF EXISTS idx_customers_full_name_trgm;
DROP INDEX IF EXISTS idx_orders_shipping_name_trgm;
DROP INDEX IF EXISTS idx_orders_email_trgm;
DROP INDEX IF EXISTS idx_orders_number_trgm;
