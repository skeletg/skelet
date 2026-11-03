-- migrate:up
-- The date every billing boundary is counted from. Adding an interval to the previous boundary loses the day of the
-- month for good the first time a short month clamps it (Jan 31 + 1 month = Mar 3), so a subscription started on the
-- 29th-31st drifted and skipped a month. The next boundary is the nearest one the existing schedule already has
ALTER TABLE public.subscriptions ADD COLUMN billing_anchor timestamp with time zone;
UPDATE public.subscriptions SET billing_anchor = next_billing_at;
ALTER TABLE public.subscriptions ALTER COLUMN billing_anchor SET NOT NULL;

-- migrate:down
ALTER TABLE public.subscriptions DROP COLUMN billing_anchor;
