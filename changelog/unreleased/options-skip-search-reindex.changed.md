- **Configurator edits no longer re-index products in search.** Changing a product's options, a template or a product
  type's templates used to re-index every product it reached, though search holds none of it - a template on a
  product type with thousands of products meant thousands of re-indexes. Caches and webhooks still hear about each
  product, now from batched reads.
