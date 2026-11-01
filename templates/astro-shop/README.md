# astro-shop

A small, fast storefront on Skelet's front GraphQL API: home (every product), product pages, cart and a
one-page checkout. Astro 7 with the Node adapter, React components rendered on the server only, Tailwind 4,
and Motion's mini `animate()` for scroll reveals.

## How a page is served

Every page renders on demand, then the catalog pages sit in Astro's route cache (`memoryCache`) until the
backend purges them. The backend POSTs `{tags, paths, warm}` to `/api/revalidate` with
`X-Revalidate-Secret` (the same contract nextjs-vetrina implements). Pages tag themselves the way the backend
purges (`core/internal/cacheinvalidate`):

| Page | Tags |
|---|---|
| `/`, `/sitemap.xml` | `catalog:browse`, `config:public` |
| `/<product-url>` | `product:<id>`, `product:<variant id>` for each variant, `config:public` |
| `/cart`, `/checkout`, `/order-placed` | never cached |

The route cache only runs in a build (`pnpm build && pnpm start`), never under `astro dev`. In dev the backend
doesn't purge localhost storefronts, so test a purge by POSTing to `/api/revalidate` yourself.

## No JavaScript required

Add to cart, quantity changes and checkout are plain forms that post and redirect. The two scripts on a page
are the cart-count badge (inline, reading the `cart_count` cookie so cached HTML stays the same for everyone)
and the motion script (about 4 KB gzipped). Cross-document view transitions are CSS-only and stay on the
browsing pages.

## Commands

```bash
skelet pnpm astro-shop run codegen     # GraphQL types from the live front schema
skelet pnpm astro-shop run typecheck
skelet pnpm astro-shop run test        # Vitest: lib, components, motion, pages (Container API) - no backend
skelet test storefront astro-shop      # Playwright against the running dev container - places real orders
```

## Not built yet

Each one is marked with a `shortcut:` comment where it applies: door delivery only (no pickup points),
offline payment methods only (no in-browser card confirmation), no bundles or products with custom
options, product descriptions shown as plain text, and only the first 100 products on the home page and in the
sitemap.
