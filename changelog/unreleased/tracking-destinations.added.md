- **Shop events can be sent server to server to Google Analytics 4, Google Ads, Meta, Microsoft
  Advertising, CJ, Awin, Klaviyo, Attentive, OpenAI Ads and your own webhook.** Every destination is in
  development: none has run against a production account yet, so each is off until enabled with
  `skelet cli plugin enable <slug>`.
  - GA4 uses the Measurement Protocol, sent to Google, its EU endpoint or your server-side Google
    Tag Manager container, with a validate-only mode for setting up.
  - Google Ads gets purchases as conversions through Google's Data Manager API, with the ad click
    id and the shopper's hashed email and phone (enhanced conversions). No Google script loads in
    the browser.
  - Meta uses the Conversions API. Email and phone are hashed as Meta requires, and the event id is
    shared with the browser pixel so nothing counts twice.
  - Microsoft Advertising uses the UET Conversions API, and OpenAI Ads its Conversions API. OpenAI's
    `oppref` click is now kept like the other click ids.
  - CJ and Awin report sales with the click their cookie kept, and follow the per-network
    "necessary" setting.
  - Klaviyo and Attentive need the personalization category. They never receive an email that
    hasn't opted into marketing, and never a phone number.
  - The webhook posts each event as JSON, signed like the store's webhooks.
  - Each needs the visitor's matching cookie choice: analytics for GA4 and the webhook, advertising
    for the ad and affiliate platforms, personalization for Klaviyo and Attentive.
  - Set them up with `skelet cli tracking add` and check one with `skelet cli tracking test`. The
    dashboard pages come later.
- **Plugins can add their own tracking destinations** through the SDK's `RegisterDestination`.
