- **Plugins can add fields to core's record pages.** `registerDetailField` puts a text, number, checkbox, select or
  date field on a product, customer, collection, country, currency, location or locale page, stored in the record's
  metadata. It saves with the page, shows under its own label in the save dialog, and counts as unsaved work like
  any other field.
- **Plugins can add columns to the customers and orders lists.** Both lists now take list extensions like the
  products list, and `metadataColumn` shows a metadata key as a column. The products and customers lists load each
  row's metadata for it.
