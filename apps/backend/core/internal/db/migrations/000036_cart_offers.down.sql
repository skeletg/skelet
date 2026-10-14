ALTER TABLE public.order_lines DROP COLUMN IF EXISTS is_custom;

ALTER TABLE public.cart_lines
    DROP COLUMN IF EXISTS tax_class,
    DROP COLUMN IF EXISTS is_custom,
    DROP COLUMN IF EXISTS price_override;

DROP INDEX IF EXISTS public.idx_carts_offer_expires;

-- Offers would collide with their customer's shopper cart under the old index
DELETE FROM public.carts WHERE kind = 'offer';

DROP INDEX IF EXISTS public.idx_carts_customer_channel_active;
CREATE UNIQUE INDEX idx_carts_customer_channel_active ON public.carts USING btree (customer_id, channel_id) WHERE (status = 'active'::text);

ALTER TABLE public.carts
    DROP CONSTRAINT IF EXISTS carts_offer_locks_check,
    DROP CONSTRAINT IF EXISTS carts_offer_expiry_check,
    DROP CONSTRAINT IF EXISTS carts_kind_check,
    DROP COLUMN IF EXISTS order_id,
    DROP COLUMN IF EXISTS offer_created_by,
    DROP COLUMN IF EXISTS offer_expired_notified_at,
    DROP COLUMN IF EXISTS offer_revoked_at,
    DROP COLUMN IF EXISTS offer_opened_at,
    DROP COLUMN IF EXISTS offer_send_count,
    DROP COLUMN IF EXISTS offer_sent_at,
    DROP COLUMN IF EXISTS offer_link_version,
    DROP COLUMN IF EXISTS offer_payment_methods,
    DROP COLUMN IF EXISTS offer_locks,
    DROP COLUMN IF EXISTS offer_show_in_account,
    DROP COLUMN IF EXISTS offer_message,
    DROP COLUMN IF EXISTS offer_expires_at,
    DROP COLUMN IF EXISTS kind;
