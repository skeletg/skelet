-- Lookups on the order and checkout paths
CREATE INDEX IF NOT EXISTS gift_cards_issued_order_idx ON public.gift_cards USING btree (issued_order_id) WHERE issued_order_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS store_credit_entries_order_idx ON public.store_credit_entries USING btree (order_id) WHERE order_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_order_lines_product_sku ON public.order_lines USING btree (product_sku);

-- Carries the summed columns so the checkout stock sum reads the index alone
CREATE INDEX IF NOT EXISTS idx_inv_res_item_location_time_cover ON public.inventory_reservations USING btree (inventory_item_id, location_id, created_at) INCLUDE (quantity_delta, reason);
DROP INDEX IF EXISTS public.idx_inv_res_item_location_time;
