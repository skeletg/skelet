- **A product option can add its own line: an add-on that follows the item.** An option
  choice backed by a product now has **Sell it as its own line**. Picked, it goes into the
  cart and onto the order as a separate line under the item - a protection plan, an
  installation - with its own SKU, price, stock and tax class, instead of a surcharge inside
  the item's price. It keeps step with the item's quantity, leaves when the item does, moves
  with it when carts merge, can be removed or refunded on its own, and coupons leave it
  alone unless the choice says otherwise. Both storefront templates and the dashboard show it
  under its item. API: `soldAsLine` and `linePromotions` on option values, `parentLineId` on
  cart and order lines. See the add-on lines note in
  [docs/commerce-model.md](../../docs/commerce-model.md).
