-- Short random credentials replace newly generated signed offer links. Keep the
-- link version so replacing a link also revokes any previously emailed signed link.
ALTER TABLE public.carts ADD COLUMN offer_link_token text DEFAULT '' NOT NULL;

UPDATE public.carts SET offer_link_token = translate(rtrim(encode(public.gen_random_bytes(16), 'base64'), '='), '+/', '-_')
WHERE kind = 'offer';

ALTER TABLE public.carts ADD CONSTRAINT carts_offer_token_check
    CHECK (kind <> 'offer' OR offer_link_token ~ '^[A-Za-z0-9_-]{22}$');
CREATE UNIQUE INDEX idx_carts_offer_token ON public.carts (offer_link_token) WHERE kind = 'offer';
