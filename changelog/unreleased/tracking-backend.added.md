- **A typed tracking contract for storefronts.** `skelet gen` writes `@skelet/storefront-core/track/contract.gen`
  from the Go that enforces it: every event name core takes and which of them a browser may send, GA4's ecommerce
  and item shapes, the click ids and the consent cookie's pattern. `pushEvent` reports a moment to `dataLayer` in
  GA4's shape and `eventAttrs` puts the same event on server-rendered markup. `skelet check codegen` fails when the
  two sides drift, which replaces the old hand-kept SYNC checks for consent and click ids.
