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
