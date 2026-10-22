- **Vetrina shows and reports money at each currency's own precision.** Prices for a currency with
  zero or three decimals (JPY, BHD) were divided by a hundred everywhere. Product structured data,
  `view_item`, `begin_checkout` and `purchase` events and code-only price formatting now use the
  store's configured decimals, or ISO 4217's when only a currency code is known. Order pages
  convert with the order's own currency.
