- `productsByIds` returned every product without a name, and the admin cache spread that blank over
  names other queries had loaded - recommendations showed SKUs where names belonged.
- Opening a product picker on a product page no longer reloads the page and closes the picker. The
  picker's type filter replaced the page's cached type attributes with a narrower copy.
- Recommendations past the first 100 linked products (across all three lists) showed as
  "Unavailable product" - `productsByIds` answers at most 100 ids, and the editor asked for all of
  them at once. It now asks in batches.
