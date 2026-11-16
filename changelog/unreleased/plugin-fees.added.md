- **Plugins can charge fees of their own.** `RegisterFee` adds a charge to the order, priced after shipping and
  before tax: cash-on-delivery handling, a payment surcharge, an eco fee. Core taxes it under the fee's tax class
  unless it's untaxed, and a remote tax provider gets the fees in its request. The checkout quote prices fees the
  way placement does, so `CheckoutTotals.fees` shows the shopper what they'll pay, and an `sdk.UserError` from a fee
  stops the order with its message. The order keeps each fee with its label and tax, as `Order.fees` on both APIs,
  on the invoice, in the order emails and in the dashboard, and an edit re-taxes it without asking the plugin again.
