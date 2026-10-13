ALTER TABLE public.back_in_stock_requests
    ADD COLUMN product_name text DEFAULT ''::text NOT NULL,
    ADD COLUMN product_image text DEFAULT ''::text NOT NULL,
    ADD COLUMN product_url text DEFAULT ''::text NOT NULL;
