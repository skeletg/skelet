-- FIXED option surcharges are bare minor units of the product's currency, so a channel selling in another currency needs its own amount
CREATE TABLE public.catalog_product_option_value_channel_prices (
    value_id uuid NOT NULL,
    channel_id uuid NOT NULL,
    price_value integer NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT catalog_product_option_value_channel_prices_pkey PRIMARY KEY (value_id, channel_id),
    CONSTRAINT catalog_product_option_value_channel_prices_price_check CHECK (price_value BETWEEN -1000000000 AND 1000000000),
    CONSTRAINT catalog_product_option_value_channel_prices_value_id_fkey FOREIGN KEY (value_id)
        REFERENCES public.catalog_product_option_values(id) ON DELETE CASCADE,
    CONSTRAINT catalog_product_option_value_channel_prices_channel_id_fkey FOREIGN KEY (channel_id)
        REFERENCES public.channels(id) ON DELETE CASCADE
);

CREATE INDEX idx_option_value_channel_prices_channel ON public.catalog_product_option_value_channel_prices USING btree (channel_id);

CREATE TABLE public.catalog_product_option_channel_prices (
    option_id uuid NOT NULL,
    channel_id uuid NOT NULL,
    price_value integer NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT catalog_product_option_channel_prices_pkey PRIMARY KEY (option_id, channel_id),
    CONSTRAINT catalog_product_option_channel_prices_price_check CHECK (price_value BETWEEN -1000000000 AND 1000000000),
    CONSTRAINT catalog_product_option_channel_prices_option_id_fkey FOREIGN KEY (option_id)
        REFERENCES public.catalog_product_options(id) ON DELETE CASCADE,
    CONSTRAINT catalog_product_option_channel_prices_channel_id_fkey FOREIGN KEY (channel_id)
        REFERENCES public.channels(id) ON DELETE CASCADE
);

CREATE INDEX idx_option_channel_prices_channel ON public.catalog_product_option_channel_prices USING btree (channel_id);
