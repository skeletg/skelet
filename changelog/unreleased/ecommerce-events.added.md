- **The full GA4 ecommerce event set.** Tracking destinations now also get `login`, `add_to_wishlist`,
  `generate_lead` (a signup form sent), `search`, `view_promotion` and `select_promotion`, and a cart
  quantity change becomes `add_to_cart` or `remove_from_cart` for the difference.
  - Shipping and payment choices are reported by the backend as `add_shipping_info` and
    `add_payment_info`, with `shipping_tier` and `payment_type`. Browsers can no longer send them.
  - A `purchase` names the coupons the order redeemed. Shop events also carry GA4's list, position,
    promotion and search fields, and GA4 and Meta pass them on.
  - Vetrina reports list views and clicks, cart views, searches and product-page promotions.
  - Signals now records refunds, wishlist changes, checkout choices and leads under its own names.
