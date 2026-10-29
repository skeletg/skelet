- **A bundle can no longer go into the cart empty.** When every option was optional and the
  customer cleared them all, the bundle went in as a 0.00 line that checkout let through.
  Add-to-cart now refuses it with `BUNDLE_SELECTION_MISSING`.
- **A deleted bundle no longer goes through checkout.** If the bundle product was deleted
  while it sat in a cart, checkout used to place the order anyway. It now refuses the line
  with `PRODUCT_NOT_PUBLISHED`.
- **Checkout refuses a product that left the store.** A product deleted or unpublished
  while it sat in a cart used to be ordered anyway. Checkout now refuses the line with
  `PRODUCT_NOT_PUBLISHED`, the same as the add-to-cart check.
- **Checkout re-checks a bundle's picks against the bundle as it is now.** If a picked option
  or selection was removed after the bundle went into the cart, or a newly required option or
  tighter limit means the picks no longer fit, checkout now refuses the line instead of
  placing an order the bundle no longer allows.
- **A bundle in the cart follows its components' prices.** A bundle used to keep the price
  it had when added, so checkout charged a stale total after a component's price changed.
  Checkout now re-sums the components at today's prices and quantities per bundle, and the
  cart's `currentUnitPrice` flags the change beforehand, as it already did for simple
  products. A price set by a `cart.bundle.adding` hook is kept.
- **Bundle errors on the storefront no longer show internal ids.** Refused bundle adds and
  checkouts now show a plain message instead of the backend text with option and selection
  UUIDs. Checkout reports a bundle that changed after it was added as `BUNDLE_CHANGED`.
- **A bundle whose component is out of stock is refused when it's added.** The stock check
  used to look at the bundle itself, which holds no stock, so a short component only failed
  the order at placement. Adding a bundle or raising its quantity now checks every component,
  the same way a simple product is checked.
- **A bundle no longer picks a default the store doesn't sell.** When a customer left an
  option untouched, every default was added, including ones not listed on that store, and
  the add failed over a product they never saw. Defaults the store doesn't sell are now
  skipped.
- **A bundle's in-stock flag matches what can actually be added.** It used to count products
  not listed on the store, accepted one available pick for an option that needs two, and
  showed a bundle as in stock when nothing in it could be bought. It now applies the same
  rules as add-to-cart.
- **Retrying a failed bundle save no longer duplicates what already went through.** When one
  change in a bundle save failed partway, the ones before it were saved but stayed queued,
  so the retry sent them again. A retry now sends only the changes that didn't go through.
- **Bundle option titles are edited in the language you're viewing.** Editing a title while
  the product page showed a translation used to write it into whichever language was stored
  first, often overwriting the default. The dialog now reads and saves the page's language,
  and typing a title for a language that has none yet adds that translation.
