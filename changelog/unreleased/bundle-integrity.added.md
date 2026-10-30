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
- **The bundle picker adds all of a configurable's variants in one pick.** Configurables are
  offered beside simple products, and picking one stages each variant not already in the
  option as its own selection, with the row's quantity.
- **A bundle selection can be a configurable product whose variant the customer picks.**
  "A hoodie, any size" is one selection. The storefront asks for the size once the hoodie is
  picked, and the cart line prices, snapshots and reserves that size. A size added to the
  hoodie later is offered without editing the bundle. In the picker, a configurable stays one
  selection unless Each variant is ticked. A configurable selection can't be a default. The
  storefront API takes the choice as `AddBundleToCartInput.variants`, and a missing one is
  refused with `BUNDLE_VARIANT_MISSING`.
- **A bundle option can fill itself from a category.** Edit an option, turn on "Fill from a
  category" and pick the category. You can narrow it to products holding chosen attribute
  values and set a quantity for each. Every simple product it matches is offered after the
  hand-picked selections, and products joining the category later show up without an edit.
  The rows it makes are marked "from the rule" and can't be edited one by one. Their ids stay
  the same, so carts, checkout repricing and reorders treat them like hand-picked ones. API:
  `BundleOption.rule`, `BundleSelection.fromRule`, and `setRule` in `setBundleStructure`.
- **Bundle selections can carry an add-on deal and a customer-set quantity.** Each selection
  row has a "% off" field, which the cart takes off that component's price, and a "Customer
  sets qty" switch. With the switch on, the storefront asks "How many" when the item is
  picked, starting from the selection's quantity. The price line, the storefront price, the
  cart, checkout repricing and reorder all follow both. API: `discountPercent` and
  `customerQuantity` on `BundleSelection` and its inputs, and
  `AddBundleToCartInput.quantities`.
- **A cart can hold one bundle in several configurations.** A bundle line is keyed by its
  picks: a box with different contents becomes a line of its own, while re-adding the same
  picks updates the line already holding them. Simple and configured lines keep one line per
  SKU. Merging a guest cart, repricing and quantity edits work per configuration.
