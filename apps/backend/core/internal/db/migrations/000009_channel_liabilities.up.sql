-- A channel with outstanding gift cards cannot be deleted out from under them
ALTER TABLE ONLY public.gift_cards DROP CONSTRAINT gift_cards_channel_id_fkey;
ALTER TABLE ONLY public.gift_cards
    ADD CONSTRAINT gift_cards_channel_id_fkey FOREIGN KEY (channel_id) REFERENCES public.channels(id) ON DELETE RESTRICT;

UPDATE public.gift_cards g
SET    channel_id = o.channel_id
FROM   public.orders o
WHERE  g.issued_order_id = o.id AND g.channel_id IS NULL;

INSERT INTO public.product_review_summaries (product_id, channel_id, is_stale, updated_at)
SELECT DISTINCT r.product_id, r.channel_id, true, now()
FROM   public.reviews r
WHERE  r.status = 'approved'
ON CONFLICT (product_id, channel_id) DO NOTHING;
