- **Shop events can go server to server to analytics and ad platforms.** Purchases, refunds,
  cancellations, cart adds and removals, and sign-ups are reported by the backend itself. Product
  views and checkout steps come from the storefront through its own `/t/e` path, so no third-party
  script is involved.
  - Each event carries the GA4 ecommerce payload, with the SKU as `item_id`.
  - Each platform account is a destination, set per shop. An event reaches it only where the
    visitor's cookie choices allow; orders use the choice made at purchase.
  - Delivery is queued with retries. One failing event never holds up the rest, and secrets are
    kept out of the delivery log.
  - It works with Signals switched off.
  - The platforms themselves arrive as plugins in a later release.
