- **The Next storefront can take card payments.** A payment plugin registers its card fields with
  `registerPaymentUI`, and checkout shows them under its method: the gateway makes a single-use
  token when the shopper presses Pay, the order is placed with it, and an in-page step like 3DS
  runs right after, before the gateway itself confirms the payment and the order page loads. A
  gateway that redirects to its own checkout gets the browser sent there. See "The frontend
  halves" in `docs/plugins.md`.
- **`PAYMENT_ORIGINS`** names a card gateway's hosts for the storefront's CSP at build. The
  backend, dashboard and Caddy CSPs already take them from the compiled-in plugin.
- The storefront API's `Order.paymentActionKind` says what `paymentClientSecret` is for:
  `challenge` or `redirect`, and `Order.paymentFailed` says the gateway refused the payment
  when the order was placed.
