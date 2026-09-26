CREATE INDEX IF NOT EXISTS idx_inv_res_item_location_time ON public.inventory_reservations USING btree (inventory_item_id, location_id, created_at);
DROP INDEX IF EXISTS public.idx_inv_res_item_location_time_cover;
DROP INDEX IF EXISTS public.idx_order_lines_product_sku;
DROP INDEX IF EXISTS public.store_credit_entries_order_idx;
DROP INDEX IF EXISTS public.gift_cards_issued_order_idx;
