- **Plugin fields on core records take a line.** `sdk.Link` with `sdk.LoadOne` or `sdk.LoadMany` reads a plugin's rows
  for a whole list of records in one query, filling its GraphQL model by the fields' names.
- **Plugins purge the storefront's pages for a record.** `sdk.InvalidateStorefront` purges a product's, category's,
  collection's or CMS page's cached pages the way core's own change to it does, for a plugin whose data on the record
  changed.
