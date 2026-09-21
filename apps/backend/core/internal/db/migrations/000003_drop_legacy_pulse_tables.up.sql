--
-- A database created by an earlier baseline still carries Pulse - campaigns, automation flows,
-- the email builder, the send log and the suppression list - which live in the signals database
-- now and are the Signals module's alone. Nothing in core reads them any more: receipts and
-- account mail render from templates compiled into the email package, and every other read
-- goes over the wire contract
--
-- A fresh install has none of these, so this is a no-op there. Rows do not travel: marketing
-- state is what a new release starts clean on
--
DROP TABLE IF EXISTS public.email_campaign_recipients;
DROP TABLE IF EXISTS public.email_campaigns;
DROP TABLE IF EXISTS public.flow_runs;
DROP TABLE IF EXISTS public.flows;
DROP TABLE IF EXISTS public.email_suppressions;
DROP TABLE IF EXISTS public.email_sends_default;
DROP TABLE IF EXISTS public.email_sends;
DROP FUNCTION IF EXISTS public.drop_email_send_month(date);
DROP FUNCTION IF EXISTS public.email_send_months_before(timestamp with time zone);
DROP FUNCTION IF EXISTS public.ensure_email_send_partitions(integer);
DROP TABLE IF EXISTS public.email_templates;
DROP TABLE IF EXISTS public.email_blocks;
DROP TABLE IF EXISTS public.email_library_revisions;
DROP TABLE IF EXISTS public.email_library;
DROP TABLE IF EXISTS public.email_groups;
