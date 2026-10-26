-- migrate:up
-- A picture on an option (an illustration beside its question) and on each choice (shown as a tile to pick) - a
-- media-library asset, cleared when the asset is deleted
ALTER TABLE public.catalog_product_options
    ADD COLUMN image_asset_id uuid REFERENCES public.media_assets(id) ON DELETE SET NULL;
ALTER TABLE public.catalog_product_option_values
    ADD COLUMN image_asset_id uuid REFERENCES public.media_assets(id) ON DELETE SET NULL;

CREATE INDEX idx_catalog_product_options_image ON public.catalog_product_options (image_asset_id) WHERE image_asset_id IS NOT NULL;
CREATE INDEX idx_catalog_product_option_values_image ON public.catalog_product_option_values (image_asset_id) WHERE image_asset_id IS NOT NULL;

-- migrate:down
DROP INDEX public.idx_catalog_product_option_values_image;
DROP INDEX public.idx_catalog_product_options_image;
ALTER TABLE public.catalog_product_option_values DROP COLUMN image_asset_id;
ALTER TABLE public.catalog_product_options DROP COLUMN image_asset_id;
