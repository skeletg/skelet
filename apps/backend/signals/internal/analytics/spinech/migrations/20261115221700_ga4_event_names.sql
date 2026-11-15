-- migrate:up
-- A browser's purchase goes: the server's order is now the one purchase on the spine (order_placed is renamed to it
-- below), and revenue never came from the browser's copy, so keeping it would count every order it saw twice. The
-- plan refuses new ones. Synchronous, so the rename below never meets a row this is still removing
ALTER TABLE analytics_events DELETE WHERE event_name = 'purchase' AND source = 'web' SETTINGS mutations_sync = 2;

-- migrate:down
-- The browser purchases removed above don't come back

-- migrate:up
-- One name per moment: GA4's where it has one, the same names the tracking plugins and any front-end tag use. A
-- promoted browser event and a custom one carry their name in properties too, so that is rewritten with it -
-- custom_name, materialized from it, follows. Signals' Postgres definitions are renamed in the same release
ALTER TABLE analytics_events UPDATE
    event_name = transform(event_name,
        ['cart_add', 'cart_remove', 'cart_update', 'checkout_shipping', 'checkout_payment', 'wishlist_add', 'wishlist_remove',
         'order_placed', 'order_refunded', 'registered', 'lead', 'password_reset', 'product_filter'],
        ['add_to_cart', 'remove_from_cart', 'cart_updated', 'add_shipping_info', 'add_payment_info', 'add_to_wishlist', 'remove_from_wishlist',
         'purchase', 'refund', 'sign_up', 'generate_lead', 'password_reset_requested', 'filter_items'],
        event_name),
    properties = if(source = 'web' AND custom_name IN ('product_filter', '$search', 'search_overview_click', 'search_overview_chat'),
        replaceOne(properties, concat('"name":"', custom_name, '"'), concat('"name":"',
            transform(custom_name, ['product_filter', '$search', 'search_overview_click', 'search_overview_chat'],
                ['filter_items', 'search', 'select_overview_item', 'open_overview_chat'], custom_name), '"')),
        properties)
WHERE event_name IN ('cart_add', 'cart_remove', 'cart_update', 'checkout_shipping', 'checkout_payment', 'wishlist_add', 'wishlist_remove',
                     'order_placed', 'order_refunded', 'registered', 'lead', 'password_reset', 'product_filter')
   OR (source = 'web' AND custom_name IN ('$search', 'search_overview_click', 'search_overview_chat'))
SETTINGS mutations_sync = 2;

-- migrate:down
ALTER TABLE analytics_events UPDATE
    event_name = transform(event_name,
        ['add_to_cart', 'remove_from_cart', 'cart_updated', 'add_shipping_info', 'add_payment_info', 'add_to_wishlist', 'remove_from_wishlist',
         'purchase', 'refund', 'sign_up', 'generate_lead', 'password_reset_requested', 'filter_items'],
        ['cart_add', 'cart_remove', 'cart_update', 'checkout_shipping', 'checkout_payment', 'wishlist_add', 'wishlist_remove',
         'order_placed', 'order_refunded', 'registered', 'lead', 'password_reset', 'product_filter'],
        event_name),
    properties = if(source = 'web' AND custom_name IN ('filter_items', 'search', 'select_overview_item', 'open_overview_chat'),
        replaceOne(properties, concat('"name":"', custom_name, '"'), concat('"name":"',
            transform(custom_name, ['filter_items', 'search', 'select_overview_item', 'open_overview_chat'],
                ['product_filter', '$search', 'search_overview_click', 'search_overview_chat'], custom_name), '"')),
        properties)
WHERE event_name IN ('add_to_cart', 'remove_from_cart', 'cart_updated', 'add_shipping_info', 'add_payment_info', 'add_to_wishlist', 'remove_from_wishlist',
                     'purchase', 'refund', 'sign_up', 'generate_lead', 'password_reset_requested', 'filter_items')
   OR (source = 'web' AND custom_name IN ('search', 'select_overview_item', 'open_overview_chat'))
SETTINGS mutations_sync = 2;
