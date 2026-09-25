-- A card with a channel spends only there, and a NULL one on every channel
ALTER TABLE public.gift_cards ADD COLUMN channel_id uuid;
ALTER TABLE ONLY public.gift_cards
    ADD CONSTRAINT gift_cards_channel_id_fkey FOREIGN KEY (channel_id) REFERENCES public.channels(id) ON DELETE CASCADE;
CREATE INDEX gift_cards_channel_idx ON public.gift_cards USING btree (channel_id) WHERE (channel_id IS NOT NULL);

DELETE FROM public.product_review_summaries;
ALTER TABLE public.product_review_summaries ADD COLUMN channel_id uuid NOT NULL;
ALTER TABLE ONLY public.product_review_summaries DROP CONSTRAINT product_review_summaries_pkey;
ALTER TABLE ONLY public.product_review_summaries
    ADD CONSTRAINT product_review_summaries_pkey PRIMARY KEY (product_id, channel_id);
ALTER TABLE ONLY public.product_review_summaries
    ADD CONSTRAINT product_review_summaries_channel_id_fkey FOREIGN KEY (channel_id) REFERENCES public.channels(id) ON DELETE CASCADE;
DROP INDEX public.product_review_summaries_stale_idx;
CREATE INDEX product_review_summaries_stale_idx ON public.product_review_summaries USING btree (updated_at) WHERE is_stale;

CREATE INDEX reviews_product_channel_approved_idx ON public.reviews USING btree (product_id, channel_id) WHERE (status = 'approved'::text);
