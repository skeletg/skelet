- **A kit is checked against its components' stock at add-to-cart.** A kit owns no stock, so
  the cart used to accept any quantity and only checkout refused it. Adding or raising a kit
  line now refuses with `CART_INSUFFICIENT_STOCK` when a component can't cover it.
- **An empty kit can no longer be bought.** A kit with no components showed as in stock and
  went into the cart as an empty box. Add-to-cart and checkout now refuse it with
  `PRODUCT_NOT_PURCHASABLE`, and the storefront shows it out of stock.
