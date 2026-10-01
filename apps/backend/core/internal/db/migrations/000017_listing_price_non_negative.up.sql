-- A negative override priced the product below zero on that channel; inheriting is the safe repair
UPDATE catalog_product_channel_listings SET price = NULL WHERE price < 0;

ALTER TABLE catalog_product_channel_listings
    ADD CONSTRAINT catalog_product_channel_listings_price_check CHECK (price IS NULL OR price >= 0);
