DELETE FROM public.back_in_stock_requests t WHERE NOT EXISTS (SELECT 1 FROM public.channels c WHERE c.id = t.channel_id);
DELETE FROM public.chat_conversations t WHERE NOT EXISTS (SELECT 1 FROM public.channels c WHERE c.id = t.channel_id);
DELETE FROM public.signup_form_submissions t WHERE NOT EXISTS (SELECT 1 FROM public.channels c WHERE c.id = t.channel_id);
DELETE FROM public.signup_forms t WHERE NOT EXISTS (SELECT 1 FROM public.channels c WHERE c.id = t.channel_id);
DELETE FROM public.web_form_submissions t WHERE NOT EXISTS (SELECT 1 FROM public.channels c WHERE c.id = t.channel_id);
DELETE FROM public.web_forms t WHERE NOT EXISTS (SELECT 1 FROM public.channels c WHERE c.id = t.channel_id);
DELETE FROM public.admin_notifications t WHERE t.channel_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM public.channels c WHERE c.id = t.channel_id);

ALTER TABLE ONLY public.back_in_stock_requests
    ADD CONSTRAINT back_in_stock_requests_channel_id_fkey FOREIGN KEY (channel_id) REFERENCES public.channels(id) ON DELETE CASCADE;
ALTER TABLE ONLY public.chat_conversations
    ADD CONSTRAINT chat_conversations_channel_id_fkey FOREIGN KEY (channel_id) REFERENCES public.channels(id) ON DELETE CASCADE;
ALTER TABLE ONLY public.signup_forms
    ADD CONSTRAINT signup_forms_channel_id_fkey FOREIGN KEY (channel_id) REFERENCES public.channels(id) ON DELETE CASCADE;
ALTER TABLE ONLY public.signup_form_submissions
    ADD CONSTRAINT signup_form_submissions_channel_id_fkey FOREIGN KEY (channel_id) REFERENCES public.channels(id) ON DELETE CASCADE;
ALTER TABLE ONLY public.web_forms
    ADD CONSTRAINT web_forms_channel_id_fkey FOREIGN KEY (channel_id) REFERENCES public.channels(id) ON DELETE CASCADE;
ALTER TABLE ONLY public.web_form_submissions
    ADD CONSTRAINT web_form_submissions_channel_id_fkey FOREIGN KEY (channel_id) REFERENCES public.channels(id) ON DELETE CASCADE;
ALTER TABLE ONLY public.admin_notifications
    ADD CONSTRAINT admin_notifications_channel_id_fkey FOREIGN KEY (channel_id) REFERENCES public.channels(id) ON DELETE CASCADE;

-- A job scoped to a channel that is gone must not survive as an all-channels one
ALTER TABLE ONLY public.data_jobs DROP CONSTRAINT data_jobs_channel_id_fkey;
ALTER TABLE ONLY public.data_jobs
    ADD CONSTRAINT data_jobs_channel_id_fkey FOREIGN KEY (channel_id) REFERENCES public.channels(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.promotion_rules DROP CONSTRAINT promotion_rules_slug_key;
ALTER TABLE ONLY public.promotion_rules
    ADD CONSTRAINT promotion_rules_channel_slug_key UNIQUE NULLS NOT DISTINCT (channel_id, slug);
ALTER TABLE ONLY public.remote_providers DROP CONSTRAINT remote_providers_kind_slug_key;
ALTER TABLE ONLY public.remote_providers
    ADD CONSTRAINT remote_providers_channel_kind_slug_key UNIQUE NULLS NOT DISTINCT (channel_id, kind, slug);

-- Feeds the abandonment index; a line write bumps it at most once a minute, keeping concurrent writers off the cart row's lock
ALTER TABLE public.carts ADD COLUMN last_activity_at timestamp with time zone DEFAULT now() NOT NULL;
UPDATE public.carts c
SET last_activity_at = GREATEST(c.updated_at, COALESCE((SELECT max(l.updated_at) FROM public.cart_lines l WHERE l.cart_id = c.id), c.updated_at));

CREATE FUNCTION public.carts_touch_last_activity() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    NEW.last_activity_at := GREATEST(NEW.last_activity_at, NEW.updated_at);
    RETURN NEW;
END;
$$;

CREATE FUNCTION public.cart_lines_touch_cart() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    UPDATE public.carts SET last_activity_at = GREATEST(last_activity_at, NEW.updated_at)
    WHERE id = NEW.cart_id AND last_activity_at < NEW.updated_at - interval '1 minute';
    RETURN NULL;
END;
$$;

CREATE TRIGGER carts_touch_last_activity BEFORE INSERT OR UPDATE ON public.carts
    FOR EACH ROW EXECUTE FUNCTION public.carts_touch_last_activity();
CREATE TRIGGER cart_lines_touch_cart AFTER INSERT OR UPDATE ON public.cart_lines
    FOR EACH ROW EXECUTE FUNCTION public.cart_lines_touch_cart();

CREATE INDEX idx_carts_active_last_activity ON public.carts USING btree (channel_id, last_activity_at DESC) WHERE (status = 'active'::text);
