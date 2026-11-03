-- migrate:up
-- A return line can name one component of a bundle line - its quantity then counts that component's units, its refund is
-- the component's share of the line, and only that component is restocked
ALTER TABLE public.order_return_lines
    ADD COLUMN order_line_component_id uuid REFERENCES public.order_line_components(id) ON DELETE RESTRICT;

-- migrate:down
ALTER TABLE public.order_return_lines DROP COLUMN order_line_component_id;
