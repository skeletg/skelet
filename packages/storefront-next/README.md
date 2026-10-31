# @skelet/storefront-next

The storefront logic a Next.js template builds on: everything that isn't design. Templates under `templates/` and
your storefronts under `apps/channels/` import it as `@skelet/storefront-next/<module>`, and it ships as source - Next
compiles workspace packages itself, so there's no build step.

One directory per domain - `cart/`, `checkout/`, `auth/`, `customer/`, `locale/`, `channel/` and the rest - each with
its reads, its server actions and the `*.graphql` operations they run. `graphql.ts` is the server-side client.

- **Imports are relative inside the package.** A storefront's `@/` alias would resolve against the storefront.
- **Codegen is the package's.** `skelet gen` runs `pnpm run codegen` here against the live front schema, writing the
  `*.gql.ts` beside each `*.graphql` and the shared `graphql/schema.types.ts` - never edit those by hand.
- **Tests sit beside the code**, plain modules in node and the client providers in jsdom: `pnpm test` here, or
  `skelet test`, which runs every storefront's and package's.
