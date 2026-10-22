- **Signed-in shoppers see their own prices, review state and CMS blocks in Vetrina.** Reads the
  backend caches for anonymous visitors now still carry the customer's session, so tier prices,
  review eligibility and votes and segment-personalized CMS content answer for that customer. Any
  request carrying a session, cart-offer link or review voter token skips the storefront's shared
  data cache.
