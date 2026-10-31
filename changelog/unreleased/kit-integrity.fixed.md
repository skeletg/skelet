- **A kit is checked against its components' stock at add-to-cart.** A kit owns no stock, so
  the cart used to accept any quantity and only checkout refused it. Adding or raising a kit
  line now refuses with `CART_INSUFFICIENT_STOCK` when a component can't cover it.
