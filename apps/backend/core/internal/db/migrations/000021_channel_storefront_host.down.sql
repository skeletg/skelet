DROP INDEX IF EXISTS public.idx_channels_storefront_host;
ALTER TABLE public.channels DROP COLUMN IF EXISTS storefront_host;
CREATE UNIQUE INDEX IF NOT EXISTS idx_channels_public_url ON public.channels USING btree (public_url) WHERE (public_url <> ''::text);
