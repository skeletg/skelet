-- migrate:up
-- A library email's version in another language - a shopper in that language gets it, everyone else the email itself
CREATE TABLE public.email_library_translations (
    email_id uuid NOT NULL REFERENCES public.email_library(id) ON DELETE CASCADE,
    locale text NOT NULL,
    subject text DEFAULT ''::text NOT NULL,
    preheader text DEFAULT ''::text NOT NULL,
    blocks jsonb DEFAULT '[]'::jsonb NOT NULL,
    styles jsonb DEFAULT '{}'::jsonb NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    PRIMARY KEY (email_id, locale),
    CONSTRAINT email_library_translations_locale_check CHECK (locale = lower(locale) AND locale <> '')
);

-- The storefront language a customer last signed in with, so a campaign reaches them in it
ALTER TABLE public.customer_roster ADD COLUMN locale text DEFAULT ''::text NOT NULL;

-- migrate:down
ALTER TABLE public.customer_roster DROP COLUMN locale;
DROP TABLE public.email_library_translations;
