-- Every product a line takes stock from besides (or instead of) itself: a kit's components, or a picked option value's linked source
ALTER TABLE public.order_line_kit_components RENAME TO order_line_stock_parts;
ALTER TABLE public.order_line_stock_parts RENAME CONSTRAINT order_line_kit_components_pkey TO order_line_stock_parts_pkey;
ALTER TABLE public.order_line_stock_parts RENAME CONSTRAINT order_line_kit_components_quantity_check TO order_line_stock_parts_quantity_check;
ALTER TABLE public.order_line_stock_parts RENAME CONSTRAINT order_line_kit_components_order_line_id_fkey TO order_line_stock_parts_order_line_id_fkey;
ALTER INDEX public.idx_order_line_kit_components_line RENAME TO idx_order_line_stock_parts_line;

ALTER TABLE public.order_line_stock_parts ADD COLUMN source text NOT NULL DEFAULT 'kit';
ALTER TABLE public.order_line_stock_parts ALTER COLUMN source DROP DEFAULT;
ALTER TABLE public.order_line_stock_parts ADD CONSTRAINT order_line_stock_parts_source_check CHECK (source IN ('kit', 'option'));

ALTER TABLE public.cart_line_options ADD COLUMN source_product_id uuid;
