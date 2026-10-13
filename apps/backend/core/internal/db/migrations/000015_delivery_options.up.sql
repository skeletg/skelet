-- Delivery options are what a channel offers at checkout - a named instance of a delivery kind (ship to address, store pickup, PO box or locker), with the admin's choices for the blocks the kind leaves configurable. Kinds live in code (internal/delivery), so kind is free text checked there rather than an enum
CREATE TABLE public.delivery_options (
    id uuid DEFAULT uuidv7() NOT NULL PRIMARY KEY,
    channel_id uuid NOT NULL REFERENCES public.channels(id) ON DELETE CASCADE,
    kind text NOT NULL,
    name text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    enabled boolean DEFAULT false NOT NULL,
    sort_order integer DEFAULT 0 NOT NULL,
    -- {"phone": "required", "billing_address": "optional"} - only blocks the kind marks configurable; a locked block is refused on save
    block_modes jsonb DEFAULT '{}'::jsonb NOT NULL,
    -- Where points come from for a kind that needs one - a core source slug (inventory-locations, manual-points), or "remote:<provider slug>"
    point_source text DEFAULT ''::text NOT NULL,
    -- Limits the option to one carrier's methods of its kind - empty takes every carrier quoting the kind
    carrier_slug text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE INDEX delivery_options_channel_idx ON public.delivery_options (channel_id, sort_order);

-- Pickup points (lockers, parcel shops, PO boxes) the admin enters for a channel - the manual-points source a parcel_locker option picks from when no remote network supplies them
CREATE TABLE public.delivery_points (
    id uuid DEFAULT uuidv7() NOT NULL PRIMARY KEY,
    channel_id uuid NOT NULL REFERENCES public.channels(id) ON DELETE CASCADE,
    kind text DEFAULT 'parcel_locker'::text NOT NULL,
    name text NOT NULL,
    street1 text NOT NULL,
    street2 text DEFAULT ''::text NOT NULL,
    city text NOT NULL,
    region text DEFAULT ''::text NOT NULL,
    postal_code text NOT NULL,
    country_code text NOT NULL,
    instructions text DEFAULT ''::text NOT NULL,
    hours text DEFAULT ''::text NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    sort_order integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE INDEX delivery_points_channel_idx ON public.delivery_points (channel_id, kind, country_code, sort_order);

-- Which delivery kinds a product type may ship by - NULL takes any kind the channel offers. A fridge allows ship_to_address and store_pickup, never a PO box
ALTER TABLE public.catalog_product_types ADD COLUMN delivery_kinds text[];

-- A cart's delivery choices, one per delivery group keyed by the group's key - {option, carrier, method, point}. split_delivery is the shopper's choice to pick per group when one option could cover everything
ALTER TABLE public.carts
    ADD COLUMN deliveries jsonb DEFAULT '{}'::jsonb NOT NULL,
    ADD COLUMN split_delivery boolean DEFAULT false NOT NULL;

-- none (digital only), physical or mixed - a digital-only order has no shipping address and nothing to ship
ALTER TABLE public.orders
    ADD COLUMN fulfillment_need text DEFAULT 'physical'::text NOT NULL,
    ADD CONSTRAINT orders_fulfillment_need_check CHECK (fulfillment_need IN ('none', 'physical', 'mixed'));

-- One package of an order - the lines of one delivery group leaving from one origin, with the delivery the shopper chose for the group. Point is a snapshot, so the order reads as placed after a source changes
CREATE TABLE public.order_shipments (
    id uuid DEFAULT uuidv7() NOT NULL PRIMARY KEY,
    order_id uuid NOT NULL REFERENCES public.orders(id) ON DELETE CASCADE,
    position integer NOT NULL,
    group_key text NOT NULL,
    origin_location_id uuid REFERENCES public.inventory_locations(id) ON DELETE SET NULL,
    delivery_kind text NOT NULL,
    delivery_option_id uuid REFERENCES public.delivery_options(id) ON DELETE SET NULL,
    delivery_option_name text DEFAULT ''::text NOT NULL,
    carrier_slug text DEFAULT ''::text NOT NULL,
    method_code text DEFAULT ''::text NOT NULL,
    method_name text DEFAULT ''::text NOT NULL,
    amount_cents bigint DEFAULT 0 NOT NULL,
    estimated_days integer,
    ship_to jsonb,
    point jsonb,
    -- [{"orderLineId", "sku", "quantity"}]
    lines jsonb DEFAULT '[]'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE INDEX order_shipments_order_idx ON public.order_shipments (order_id, position);

-- Built-in shipping methods name the kind they price, so a PO box method can cost differently from door delivery. units_per_charge prices every started bundle of N units again (5 per 5€: 1-5 units 5€, 6-10 units 10€) - 0 charges once per shipment
ALTER TABLE public.shipping_methods
    ADD COLUMN delivery_kind text DEFAULT 'ship_to_address'::text NOT NULL,
    ADD COLUMN units_per_charge integer DEFAULT 0 NOT NULL,
    ADD CONSTRAINT shipping_methods_delivery_kind_check CHECK (delivery_kind IN ('ship_to_address', 'parcel_locker', 'carrier_point')),
    ADD CONSTRAINT shipping_methods_units_per_charge_check CHECK (units_per_charge >= 0);

-- A method's own price for one product type - a fridge costs 30€ per unit and never ships free, while everything else pools into the method's default rate. free_shipping_eligible lets the method's free_above threshold waive this type's price too
CREATE TABLE public.shipping_method_type_rates (
    method_id uuid NOT NULL REFERENCES public.shipping_methods(id) ON DELETE CASCADE,
    product_type_id uuid NOT NULL REFERENCES public.catalog_product_types(id) ON DELETE CASCADE,
    price_cents bigint NOT NULL CHECK (price_cents >= 0),
    units_per_charge integer DEFAULT 0 NOT NULL CHECK (units_per_charge >= 0),
    free_shipping_eligible boolean DEFAULT false NOT NULL,
    PRIMARY KEY (method_id, product_type_id)
);

-- Remote point sources - a PO box or locker network plugged in over HTTP
ALTER TABLE public.remote_providers DROP CONSTRAINT remote_providers_kind_check;
ALTER TABLE public.remote_providers ADD CONSTRAINT remote_providers_kind_check CHECK ((kind = ANY (ARRAY['shipping_carrier'::text, 'tax_provider'::text, 'address_validator'::text, 'promotion_condition'::text, 'promotion_action'::text, 'flow_action'::text, 'fraud_risk'::text, 'currency_rate'::text, 'pricing'::text, 'recommendation'::text, 'reviews'::text, 'search'::text, 'customer_segments'::text, 'returns_policy'::text, 'inventory_availability'::text, 'shipping_tracking'::text, 'delivery_points'::text])));
