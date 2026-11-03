-- migrate:up
-- An agent cart is built over the storefront MCP endpoint and closes when it's handed to the shopper, whose link copies its lines into their own cart
ALTER TABLE public.carts
    ADD COLUMN source text DEFAULT 'web' NOT NULL,
    ADD COLUMN handoff_token text DEFAULT '' NOT NULL,
    ADD COLUMN handoff_expires_at timestamp with time zone,
    ADD CONSTRAINT carts_source_check CHECK (source IN ('web', 'agent')),
    ADD CONSTRAINT carts_handoff_check CHECK (
        (handoff_token = '' AND handoff_expires_at IS NULL)
        OR (source = 'agent' AND handoff_token ~ '^[A-Za-z0-9_-]{22}$' AND handoff_expires_at IS NOT NULL));
CREATE UNIQUE INDEX idx_carts_handoff_token ON public.carts (handoff_token) WHERE handoff_token <> '';

CREATE TABLE public.cart_handoff_redemptions (
    agent_cart_id uuid NOT NULL REFERENCES public.carts (id) ON DELETE CASCADE,
    cart_id uuid NOT NULL REFERENCES public.carts (id) ON DELETE CASCADE,
    redeemed_at timestamp with time zone DEFAULT now() NOT NULL,
    PRIMARY KEY (agent_cart_id, cart_id)
);
CREATE INDEX idx_cart_handoff_redemptions_cart ON public.cart_handoff_redemptions (cart_id);

-- migrate:down
DROP TABLE public.cart_handoff_redemptions;
DROP INDEX public.idx_carts_handoff_token;
ALTER TABLE public.carts
    DROP CONSTRAINT carts_handoff_check,
    DROP CONSTRAINT carts_source_check,
    DROP COLUMN handoff_expires_at,
    DROP COLUMN handoff_token,
    DROP COLUMN source;
