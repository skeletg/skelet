-- The send log moved to the commerce database - see its 000003_email_sends.
--
-- Existing rows do not travel with this migration: golang-migrate runs against one database
-- and cannot read the other. `skelet db move-email-sends` copies them across, and it must run
-- BEFORE this drops the source. The log is engagement history, and the pixel/click path only reads
-- the last 90 days, so skipping the copy costs reporting depth rather than correctness.

DROP TABLE IF EXISTS public.email_sends;
DROP FUNCTION IF EXISTS public.drop_email_send_month(date);
DROP FUNCTION IF EXISTS public.email_send_months_before(timestamp with time zone);
DROP FUNCTION IF EXISTS public.ensure_email_send_partitions(integer);
