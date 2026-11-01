# @skelet/storefront-core

A storefront's logic with no framework in it: the GraphQL client and every operation with its generated types, the
reads and the actions behind each page (cart, checkout, auth, customer, catalog, CMS and the rest), and the pure
helpers they share. Next, Astro or anything else can run it.

What it needs from the request it runs in - cookies, headers, locale, channel, a cache-aware fetch, redirects - it asks
through `request()` (`request.ts`). A framework adapter answers once, with `configureStorefront`:
`@skelet/storefront-next` is Next's. Reads memoize per request with `perRequest`, the framework's own memo where it has
one.

- **No framework imports here.** No `next`, `react`, `next-intl` or `server-only` - `skelet check` fails on one.
- **Imports are relative inside the package.** A storefront's `@/` alias would resolve against the storefront.
- **Codegen is this package's.** `skelet gen` runs `pnpm run codegen` here against the live front schema, writing the
  `*.gql.ts` beside each `*.graphql` and the shared `graphql/schema.types.ts` - never edit those by hand.
- **Tests answer `request()` themselves**, with `useTestRequest` from `testing.ts`.
