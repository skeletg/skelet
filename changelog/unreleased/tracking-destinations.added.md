- **Shop events can be sent to Google Analytics 4, Meta and your own webhook, server to server.**
  - GA4 uses the Measurement Protocol, sent to Google, its EU endpoint or your server-side Google
    Tag Manager container, with a validate-only mode for setting up.
  - Meta uses the Conversions API. Email and phone are hashed as Meta requires, and the event id is
    shared with the browser pixel so nothing counts twice.
  - The webhook posts each event as JSON, signed like the store's webhooks.
  - Each needs the visitor's matching cookie choice: analytics for GA4 and the webhook, advertising
    for Meta.
  - Set them up with `skelet cli tracking add` and check one with `skelet cli tracking test`. The
    dashboard pages come later.
- **Plugins can add their own tracking destinations** through the SDK's `RegisterDestination`
  (SDK 0.1.2).
