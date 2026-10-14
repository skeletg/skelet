-- An offer is a cart an admin builds for one recipient and sends as a link - it checks out like any cart, but it is never the customer's active cart, so the one-active-cart rule now covers shopper carts only
ALTER TABLE public.carts
    ADD COLUMN kind text DEFAULT 'shopper' NOT NULL,
    ADD COLUMN offer_expires_at timestamp with time zone,
    ADD COLUMN offer_message text DEFAULT '' NOT NULL,
    ADD COLUMN offer_show_in_account boolean DEFAULT false NOT NULL,
    -- What the recipient may not change - the parts the admin set up front
    ADD COLUMN offer_locks text[] DEFAULT '{}'::text[] NOT NULL,
    -- The payment methods the recipient may choose from - empty leaves every method the store offers
    ADD COLUMN offer_payment_methods text[] DEFAULT '{}'::text[] NOT NULL,
    -- Part of every link the cart has handed out - bumping it kills them all
    ADD COLUMN offer_link_version integer DEFAULT 1 NOT NULL,
    ADD COLUMN offer_sent_at timestamp with time zone,
    ADD COLUMN offer_send_count integer DEFAULT 0 NOT NULL,
    ADD COLUMN offer_opened_at timestamp with time zone,
    ADD COLUMN offer_revoked_at timestamp with time zone,
    ADD COLUMN offer_expired_notified_at timestamp with time zone,
    -- Who built it - informational, so no FK: the console may act for a service or integration identity with no admin row
    ADD COLUMN offer_created_by uuid,
    -- The order the cart became - set for offers, so the console can link to it
    ADD COLUMN order_id uuid REFERENCES public.orders(id) ON DELETE SET NULL,
    ADD CONSTRAINT carts_kind_check CHECK (kind IN ('shopper', 'offer')),
    ADD CONSTRAINT carts_offer_expiry_check CHECK (kind <> 'offer' OR offer_expires_at IS NOT NULL),
    ADD CONSTRAINT carts_offer_locks_check CHECK (offer_locks <@ ARRAY['lines', 'billing', 'shipping', 'delivery', 'coupons']::text[]);

DROP INDEX public.idx_carts_customer_channel_active;
CREATE UNIQUE INDEX idx_carts_customer_channel_active ON public.carts USING btree (customer_id, channel_id) WHERE (status = 'active'::text AND kind = 'shopper'::text);

CREATE INDEX idx_carts_offer_expires ON public.carts USING btree (offer_expires_at) WHERE (kind = 'offer'::text AND status = 'active'::text);

-- An admin-priced line keeps its price through repricing, and a custom line is a free-form charge with no catalog product behind it - it never ships, and carries its own tax class
ALTER TABLE public.cart_lines
    ADD COLUMN price_override boolean DEFAULT false NOT NULL,
    ADD COLUMN is_custom boolean DEFAULT false NOT NULL,
    ADD COLUMN tax_class text DEFAULT '' NOT NULL;

ALTER TABLE public.order_lines
    ADD COLUMN is_custom boolean DEFAULT false NOT NULL;
