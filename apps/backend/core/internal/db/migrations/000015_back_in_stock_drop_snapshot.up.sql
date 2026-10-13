-- The notice reads name, image and link from the channel's listing when it goes out, so the request-time copies were only ever written empty
ALTER TABLE public.back_in_stock_requests
    DROP COLUMN IF EXISTS product_name,
    DROP COLUMN IF EXISTS product_image,
    DROP COLUMN IF EXISTS product_url;
