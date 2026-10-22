- **Vetrina only takes orders it can actually get paid for.** The demo storefront ships no payment
  gateway SDK, so checkout now offers only offline methods, and it re-checks before placing the order
  that the chosen method is still eligible. If a gateway still asks for a payment action, the shopper
  lands on the order marked "created, payment not complete". Before, the storefront tried to finalize
  the payment itself and redirected as if it had succeeded. Orders fully covered by gift cards or
  store credit still need no method.
