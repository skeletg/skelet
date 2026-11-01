# @skelet/storefront-astro

The Astro adapter over `@skelet/storefront-core`. Astro has no request scope a module can reach, so `middleware.ts`
opens one: re-export it from a template's `src/middleware.ts` and every core read or action a page, an endpoint or a
component calls while the request renders answers from that request's `Astro.cookies` and headers.

```ts
// src/middleware.ts
export { storefront as onRequest } from "@skelet/storefront-astro/middleware";
```

- **Reads and actions come from the core directly** - `@skelet/storefront-core/cart/actions`, `.../channel/read` and
  the rest. Unlike Next, Astro keeps one copy of a module per server, so there are no wrappers to go through.
- **The channel** is the host's in `STOREFRONT_CHANNEL_HOSTS`, else `STOREFRONT_CHANNEL_ID`, as in the Next proxy.
- **The core's cached reads** stay in this process under their tags (`data-cache.ts`); a revalidate route drops them
  with `invalidateData(tags)`. Pages cache themselves with Astro's route cache, under tags the template picks.
- **A redirect or a not-found from the core** is thrown through the page and answered by the middleware - a 303, or a
  rewrite to `/404`. That works from a page's frontmatter or an endpoint; once a page has started streaming its status
  is sent, so a component can't redirect. A template catching errors around a core call lets these through:
  `if (isAdapterExit(err)) throw err;`.
- **Pages that vary by host.** With `STOREFRONT_CHANNEL_HOSTS` set, every response carries `Vary: Host`, so Astro's
  route cache keeps one entry per host instead of serving one channel's page to another.
