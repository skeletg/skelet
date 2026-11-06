- **Shop events can go server to server to analytics and ad platforms.** Purchases, refunds,
  cancellations, cart adds and removals, and sign-ups are reported by the backend itself. Product
  views and checkout steps come from the storefront through its own `/t/e` path, so no third-party
  script is involved.
  - Each event carries the GA4 ecommerce payload, with the SKU as `item_id`.
  - Each platform account is a destination, set per shop. An event reaches it only where the
    visitor's cookie choices allow; orders use the choice made at purchase.
  - Delivery is queued with retries. One failing event never holds up the rest, and secrets are
    kept out of the delivery log.
  - Each event is stored once, however many destinations it goes to. A sent delivery is deleted, so
    the delivery log shows only what is waiting or failed, and a failed one waits 30 days for a retry.
  - Within an hour of its last delivery, an event loses the visitor's data: email, phone, IP, click
    ids and the rest. Only its key stays, for 48 hours, so a replay can't send it twice. That covers
    the opt-out sent when a customer is erased.
  - Erasing a customer deletes their queued and failed events, found by index rather than by
    reading every event.
  - It works with Signals switched off.
  - The platforms themselves arrive as plugins in a later release.
