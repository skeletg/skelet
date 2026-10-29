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
