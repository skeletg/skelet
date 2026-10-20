- **Product and category pages make about 40% fewer database queries.** An uncached product page
  went from 32 queries to 19 and a category page from around 35 to 22. Stock is read once per
  product, the per-question review breakdown is only built when a query asks for it, and
  attribute, product type and translation metadata is shared across requests for up to 30
  seconds. Edits to translations, product type names and attribute groups can take up to 30
  seconds longer to reach the storefront.
