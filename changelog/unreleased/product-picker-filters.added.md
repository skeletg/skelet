- **The product picker and the Products list filter by category, collection, price, stock and
  attributes.** A category includes its subcategories, a price is read in each product's own
  currency, and stock follows the storefront's rule - a product with no stock record counts as in
  stock. The admin API's `productsPaged` takes the same filters as `categoryIds`, `collectionIds`,
  `priceMin` / `priceMax` and `inStock`.
