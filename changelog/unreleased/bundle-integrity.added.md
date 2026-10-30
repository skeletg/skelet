- **Bundle options can set how many items the customer picks.** Checkbox and multiselect
  options get "At least" and "At most" fields in the dashboard, so a "pick 3 of 12" box can be
  built without the API. The option card shows the rule, for example "pick 3" or "up to 2",
  and the dialog warns when the minimum is more than the option's selections.
- **The bundle editor shows what customers can buy on each store.** Above the options, one row
  per store the bundle is listed on names every selection that store won't sell (not listed,
  unpublished, out of stock or unpriced), a default customers won't get, and an option that
  leaves the bundle unbuyable. Each row also gives the price range, for example "Customers pay
  €20.00–€35.00, default €25.00". It follows staged edits before the Save.
- **Product listings in the admin API answer `inStock` per store**, so one query can check
  stock across every store a product is sold on.
- **The storefront API exposes bundle pick limits and order line components.** `BundleOption`
  gains `minSelections` and `maxSelections`, and `OrderLine` gains `components`.
- **The bundle product page keeps to the pick limits.** A "pick up to 3" option says so with a
  running count, and its other choices lock once three are picked. An option short of its
  minimum counts as unfinished, like an empty required one.
- **The order page lists what's in a bundle**, under the line, as the cart does.
- **Bundles can be priced per store: sum of the picks, a fixed price, or the sum less a
  percentage.** Each store row in the bundle editor has a pricing choice, saved with the page,
  and the price line follows it ("Customers pay €18.00–€27.00 · picks add up to €20.00–€30.00").
  The cart, checkout repricing and the order all charge the chosen price, and tax is worked out
  on it. The storefront shows the saving with the picks' sum struck through. The admin API
  takes it through `setBundlePricing`. The storefront API exposes it as
  `Product.bundlePricing` and `bundleDiscountPercent`.
