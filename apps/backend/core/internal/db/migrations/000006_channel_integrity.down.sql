DROP INDEX IF EXISTS public.idx_carts_active_last_activity;
DROP TRIGGER IF EXISTS cart_lines_touch_cart ON public.cart_lines;
DROP TRIGGER IF EXISTS carts_touch_last_activity ON public.carts;
DROP FUNCTION IF EXISTS public.cart_lines_touch_cart();
DROP FUNCTION IF EXISTS public.carts_touch_last_activity();
ALTER TABLE public.carts DROP COLUMN IF EXISTS last_activity_at;

-- A slug shared across channels keeps its global or oldest row and the rest take their channel's slug as a suffix
UPDATE public.remote_providers p
SET    slug = p.slug || '-' || c.slug
FROM   (SELECT id, row_number() OVER (PARTITION BY kind, slug ORDER BY channel_id IS NOT NULL, created_at, id) AS n FROM public.remote_providers) d,
       public.channels c
WHERE  d.id = p.id AND d.n > 1 AND c.id = p.channel_id;
ALTER TABLE ONLY public.remote_providers DROP CONSTRAINT IF EXISTS remote_providers_channel_kind_slug_key;
ALTER TABLE ONLY public.remote_providers ADD CONSTRAINT remote_providers_kind_slug_key UNIQUE (kind, slug);
UPDATE public.promotion_rules p
SET    slug = p.slug || '-' || c.slug
FROM   (SELECT id, row_number() OVER (PARTITION BY slug ORDER BY channel_id IS NOT NULL, created_at, id) AS n FROM public.promotion_rules) d,
       public.channels c
WHERE  d.id = p.id AND d.n > 1 AND c.id = p.channel_id;
ALTER TABLE ONLY public.promotion_rules DROP CONSTRAINT IF EXISTS promotion_rules_channel_slug_key;
ALTER TABLE ONLY public.promotion_rules ADD CONSTRAINT promotion_rules_slug_key UNIQUE (slug);

ALTER TABLE ONLY public.data_jobs DROP CONSTRAINT IF EXISTS data_jobs_channel_id_fkey;
ALTER TABLE ONLY public.data_jobs
    ADD CONSTRAINT data_jobs_channel_id_fkey FOREIGN KEY (channel_id) REFERENCES public.channels(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.admin_notifications DROP CONSTRAINT IF EXISTS admin_notifications_channel_id_fkey;
ALTER TABLE ONLY public.web_form_submissions DROP CONSTRAINT IF EXISTS web_form_submissions_channel_id_fkey;
ALTER TABLE ONLY public.web_forms DROP CONSTRAINT IF EXISTS web_forms_channel_id_fkey;
ALTER TABLE ONLY public.signup_form_submissions DROP CONSTRAINT IF EXISTS signup_form_submissions_channel_id_fkey;
ALTER TABLE ONLY public.signup_forms DROP CONSTRAINT IF EXISTS signup_forms_channel_id_fkey;
ALTER TABLE ONLY public.chat_conversations DROP CONSTRAINT IF EXISTS chat_conversations_channel_id_fkey;
ALTER TABLE ONLY public.back_in_stock_requests DROP CONSTRAINT IF EXISTS back_in_stock_requests_channel_id_fkey;
