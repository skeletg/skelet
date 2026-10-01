-- What a kit line took from each component at placement, per kit unit, so release and edit give back that and not today's recipe
CREATE TABLE public.order_line_kit_components (
    id uuid DEFAULT uuidv7() NOT NULL,
    order_line_id uuid NOT NULL,
    product_id uuid NOT NULL,
    quantity numeric(14,4) NOT NULL,
    sort_order integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT order_line_kit_components_pkey PRIMARY KEY (id),
    CONSTRAINT order_line_kit_components_quantity_check CHECK (quantity > 0),
    CONSTRAINT order_line_kit_components_order_line_id_fkey FOREIGN KEY (order_line_id)
        REFERENCES public.order_lines(id) ON DELETE CASCADE
);

CREATE INDEX idx_order_line_kit_components_line ON public.order_line_kit_components USING btree (order_line_id, sort_order);
