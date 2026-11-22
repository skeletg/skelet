-- migrate:up
-- A price set on a shopper's own cart is for so many of the item and for so long: the shopper may buy up to the
-- quantity it was set for at that price, and past the expiry the line goes back to its catalog price at checkout.
-- Both are null on an offer's lines, which the recipient can't change and which the offer's own expiry covers
ALTER TABLE public.cart_lines
    ADD COLUMN price_max_quantity numeric(14,4),
    ADD COLUMN price_expires_at timestamp with time zone,
    ADD CONSTRAINT cart_lines_price_max_quantity_check CHECK (price_max_quantity IS NULL OR price_max_quantity > 0);

-- migrate:down
ALTER TABLE public.cart_lines
    DROP CONSTRAINT IF EXISTS cart_lines_price_max_quantity_check,
    DROP COLUMN IF EXISTS price_expires_at,
    DROP COLUMN IF EXISTS price_max_quantity;
