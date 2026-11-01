# @skelet/storefront-next

The Next.js adapter over `@skelet/storefront-core`. `setup.ts` answers the core's questions about a request the Next way
- `next/headers`, `next/navigation`, `next/cache`, next-intl, the `[channel]` root param - and every module here
imports it first, since each server bundle layer keeps its own copy of the core.

What's left here is what only Next can do:

- **Server actions** - a `"use server"` module per domain at the path a template imports, each a thin wrapper over the
  core's action.
- **Reads** - a module per domain re-exporting the core's, so a template's imports reach them through the setup.
- **The proxy** - `storefrontProxy` routes every request to its channel and locale, rotates a near-expiry session,
  mints the visitor id, holds a `?promo=` code and reports a crawler's fetch. A template's `proxy.ts` calls it and keeps
  only the `matcher`, which Next reads from that file as written.
- **Next-only client code** - the cart provider (next-intl) and the customer provider (`next/navigation`), and the
  page metadata builder, whose shape is Next's `Metadata`.

Pure helpers come from `@skelet/storefront-core` directly; React hooks with no Next in them from
`@skelet/storefront-react`.
