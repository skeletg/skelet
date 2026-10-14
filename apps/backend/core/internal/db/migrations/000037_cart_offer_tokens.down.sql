DROP INDEX public.idx_carts_offer_token;
ALTER TABLE public.carts DROP CONSTRAINT carts_offer_token_check, DROP COLUMN offer_link_token;
