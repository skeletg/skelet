ALTER TABLE ONLY public.gift_cards DROP CONSTRAINT IF EXISTS gift_cards_channel_id_fkey;
ALTER TABLE ONLY public.gift_cards
    ADD CONSTRAINT gift_cards_channel_id_fkey FOREIGN KEY (channel_id) REFERENCES public.channels(id) ON DELETE CASCADE;
