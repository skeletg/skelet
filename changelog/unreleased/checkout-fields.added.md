- **Plugins can ask the shopper more at checkout.** `RegisterCheckoutField` declares a text, textarea, checkbox,
  select or date field, optionally required and with a `Validate` func of the plugin's own. The storefront API lists
  the fields on `CartCheckout.fields` and takes the answers with the details step, both storefront templates draw
  them, and placement refuses while a required one is empty. The answers land on the order with their labels, shown
  in the dashboard under "At checkout" and returned as `Order.checkoutFields`. On `checkout.validating` and
  `order.placing` a plugin reads them through `sdk.CheckoutFieldAnswers`, and on `order.placing` it records its own
  values with `sdk.OrderPlacingMetadata`, returned as `Order.metadata`.
