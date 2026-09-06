DROP TABLE IF EXISTS public.email_sends;
DROP FUNCTION IF EXISTS public.drop_email_send_month(date);
DROP FUNCTION IF EXISTS public.email_send_months_before(timestamp with time zone);
DROP FUNCTION IF EXISTS public.ensure_email_send_partitions(integer);
