DROP INDEX IF EXISTS public.reviews_product_channel_approved_idx;

DELETE FROM public.product_review_summaries;
DROP INDEX public.product_review_summaries_stale_idx;
CREATE INDEX product_review_summaries_stale_idx ON public.product_review_summaries USING btree (product_id) WHERE is_stale;
ALTER TABLE ONLY public.product_review_summaries DROP CONSTRAINT product_review_summaries_channel_id_fkey;
ALTER TABLE ONLY public.product_review_summaries DROP CONSTRAINT product_review_summaries_pkey;
ALTER TABLE public.product_review_summaries DROP COLUMN channel_id;
ALTER TABLE ONLY public.product_review_summaries
    ADD CONSTRAINT product_review_summaries_pkey PRIMARY KEY (product_id);

DROP INDEX public.gift_cards_channel_idx;
ALTER TABLE public.gift_cards DROP COLUMN channel_id;
