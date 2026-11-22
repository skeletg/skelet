- **An order is never placed at a total the shopper wasn't shown.** `placeOrder` takes
  `expectedTotalCents`, the total the page showed. If the order would come to anything else
  it places nothing and refuses with `CHECKOUT_TOTAL_CHANGED`, the error's extensions carrying
  the figures as they are now. Both storefront templates send it, show the new total and let
  the shopper agree to it. A storefront forked earlier keeps working unguarded until its
  payment form sends the field - see [docs/checkout.md](../../docs/checkout.md).
- **A GraphQL error can carry more than its code.** An error that implements
  `ErrorExtensions()` has its entries added to the error's extensions on both surfaces.
