- **A price a plugin or webhook sets on a cart line now holds.** Answering `cart.line.adding`
  with a unit price used to last until checkout priced the cart again from the catalog. It is
  now a set price like an admin's: it holds, the cart and order line record who set it
  (`priceSource: HOOK`, `priceRef`) and the catalog price it replaced, it is for the quantity
  priced - more of the item asks the hook again and is refused if unanswered - and coupons
  and promotions leave it alone unless the hook says they apply. `cart.line.updating` can
  price the line at its new quantity too (`unitPriceCents` in a webhook's answer,
  `SetLineUnitPrice` on `sdk.CartLineUpdating`). A hook-priced bundle keeps promotions off the
  same way. For a price that should behave like the catalog's own, a plugin registers a
  `PriceResolver` instead. See [docs/plugins.md](../../docs/plugins.md).
