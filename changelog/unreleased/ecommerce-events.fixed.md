- **Signup forms and other `/t/` requests carry the shopper's real consent.** The storefront's backend
  proxy used to pass the browser's own `X-Consent`, `X-Anon-Id`, `X-Session-Id` and `X-Click-Ids`
  through as sent. It now replaces them with values read from the storefront's cookies.
