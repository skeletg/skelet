- **Shop events can be sent server to server to Google Analytics 4, Google Ads and Meta.** Each
  platform is a plugin in its own repository - `skelet-tracking-ga4`, `skelet-tracking-google-ads` and
  `skelet-tracking-meta` under `github.com/skeletg` - added to a build with
  `skelet cli plugin add github.com/skeletg/<plugin>`. Every one is in development: none has run
  against a production account yet.
  - GA4 uses the Measurement Protocol, sent to Google, its EU endpoint or your server-side Google
    Tag Manager container, with a validate-only mode for setting up.
  - Google Ads gets purchases as conversions through Google's Data Manager API, with the ad click
    id and the shopper's hashed email and phone (enhanced conversions). No Google script loads in
    the browser.
    Refunds and cancellations restate the conversion's value. A second destination keeps a
    Customer Match list of account holders who opted into marketing, and takes them off when they
    opt out or their account is deleted or erased.
  - Meta uses the Conversions API. Email and phone are hashed as Meta requires, and the event id is
    shared with the browser pixel so nothing counts twice.
  - Each needs the visitor's matching cookie choice: analytics for GA4, advertising for Google Ads
    and Meta.
  - Set them up with `skelet cli tracking add` and check one with `skelet cli tracking test`. The
    dashboard pages come later. A secret goes in `--secret`, which is stored encrypted: a secret or
    an unknown key passed as a `--set` setting is refused.
- **Plugins can add their own tracking destinations** through the SDK's `RegisterDestination`.
