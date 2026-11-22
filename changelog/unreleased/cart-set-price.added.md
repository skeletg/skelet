- **An admin can set the price of a line in a shopper's own cart.** A price match or a
  goodwill cut no longer needs an offer: **Set price** on the cart page takes a reason and
  the new **Set Cart Prices** permission (`orders.set_cart_price`). It can't go lower than
  the channel's **Largest price cut (%)** allows, is for the quantity the line holds - the
  shopper can buy fewer at it, not more - and holds for **Set price holds (days)**. Both
  storefront templates show the line as a price set for the shopper, beside what the item
  usually costs. See [docs/cart-offers.md](../../docs/cart-offers.md).
- **The storefront API says when a line's price was set.** `CartLine` has `listUnitPrice`
  and `priceMaxQuantity`, and a quantity past it is refused with `CART_LINE_PRICE_QUANTITY`.
