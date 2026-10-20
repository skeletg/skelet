- **The backend's response cache now actually serves the storefront.** The storefront never sent
  an operation name and the cache only recognises queries by name, so no storefront read was ever
  cached there. It sends one now, and a few more public queries joined the cacheable list:
  load-more on listings, product compare, blog authors and taxonomy, the store product lookup,
  the free-shipping threshold and channel locales.
- **A storefront purge can't be refilled with the old answer any more.** Each copy the storefront
  caches names its purge surface, and a purge gives that surface a fresh epoch in the backend's
  response cache before the storefront hears about it, so the refetch always reaches current data.
- **Settings → Cache now controls the storefront.** `cache/<surface>/enabled` and `ttl_seconds`
  showed in the admin but were never read. A disabled surface now refreshes every second while
  staying purgeable, and an enabled one keeps its data for the TTL you set.
- Cached responses are stored as they were sent rather than base64 inside JSON, so the response
  cache needs 15-30% less memory per entry.
