-- Checkout state lives on the cart, so a reload resumes where the shopper was and placement reads it from here. Addresses are snapshots (the AddressInput shape) with the saved address they came from, so editing the address book never changes a checkout in progress
ALTER TABLE public.carts
    ADD COLUMN shipping_address jsonb,
    ADD COLUMN shipping_address_id uuid REFERENCES public.addresses(id) ON DELETE SET NULL,
    ADD COLUMN billing_same_as_shipping boolean DEFAULT true NOT NULL,
    ADD COLUMN billing_address jsonb,
    ADD COLUMN billing_address_id uuid REFERENCES public.addresses(id) ON DELETE SET NULL,
    ADD COLUMN shipping_carrier_slug text DEFAULT ''::text NOT NULL,
    ADD COLUMN shipping_method_code text DEFAULT ''::text NOT NULL,
    ADD COLUMN payment_provider_slug text DEFAULT ''::text NOT NULL,
    ADD COLUMN po_number text DEFAULT ''::text NOT NULL,
    ADD COLUMN use_store_credit boolean DEFAULT false NOT NULL,
    ADD COLUMN gift_card_codes text[] DEFAULT '{}'::text[] NOT NULL,
    -- Where the order is headed before a full address exists - the first step quotes delivery and pickup from a postcode alone
    ADD COLUMN destination_country text DEFAULT ''::text NOT NULL,
    ADD COLUMN destination_region text DEFAULT ''::text NOT NULL,
    ADD COLUMN destination_postal_code text DEFAULT ''::text NOT NULL,
    ADD COLUMN contact_phone text DEFAULT ''::text NOT NULL,
    ADD COLUMN purchase_type text DEFAULT 'personal'::text NOT NULL,
    ADD COLUMN company_name text DEFAULT ''::text NOT NULL,
    -- The company identifiers a business purchase gave, keyed by the country's field key ({"pib": "..."})
    ADD COLUMN business_ids jsonb DEFAULT '{}'::jsonb NOT NULL,
    -- Who collects a pickup order, {firstName, lastName, someoneElse} - null until the details step of a pickup checkout
    ADD COLUMN pickup_contact jsonb,
    ADD CONSTRAINT carts_purchase_type_check CHECK (purchase_type IN ('personal', 'business'));

-- The company identifiers on a business purchase, snapshotted with their labels so the invoice reads as the shopper saw
-- it ([{"key": "pib", "label": "PIB", "value": "..."}]) - empty for personal purchases
ALTER TABLE public.orders ADD COLUMN business_ids jsonb DEFAULT '[]'::jsonb NOT NULL;

-- Which company identifiers a business purchase from this country asks for - Serbia's PIB and Matični broj, a VAT
-- number elsewhere - as [{"key", "label", "required", "pattern", "hint"}], edited per country in the dashboard
ALTER TABLE public.countries ADD COLUMN business_id_fields jsonb DEFAULT '[]'::jsonb NOT NULL;
