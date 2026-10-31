# @skelet/storefront-next

The storefront logic a Next.js template builds on: everything that isn't design. Templates under `templates/` and
your storefronts under `apps/channels/` import it as `@skelet/storefront-next/<module>`, and it ships as source - Next
compiles workspace packages itself, so there's no build step.

Inside the package, imports are relative: a storefront's `@/` alias would resolve against the storefront.
