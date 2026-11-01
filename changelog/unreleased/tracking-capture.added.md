- **Guests get the email opt-in at checkout too.** The tick is kept under the order's email, with
  the order number as proof, and moves onto the account once someone signs up and verifies that
  address. A choice the account made later still wins. Data exports list it and erasure deletes it.
- **Ad and affiliate click ids are kept for server-side conversions.** With advertising consent, the
  storefront stores `gclid`, `gbraid`, `wbraid`, `msclkid`, `ttclid`, Awin's `awc`, CJ's `cje`
  (following CJ's cookie spec) and Meta's `_fbp`/`_fbc` in first-party cookies set by the server.
  Withdrawing consent deletes them, and a click that arrives before consent is not stored.
- **Each order records how it was placed.** The visitor's cookie consent and, only where that
  consent allows it, their visitor id, click ids, IP and browser are saved with the order. Events
  sent later then follow the consent that applied at purchase.
- **Affiliate cookies can be treated as necessary, per network.** The new privacy settings for CJ
  and Awin let their click cookie be kept without advertising consent, where a programme requires
  it. Both are off by default. `STOREFRONT_COOKIE_DOMAIN` sets the domain the click-id cookies use.
