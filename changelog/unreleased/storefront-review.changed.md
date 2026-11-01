- **Storefront prod images** install only what their template depends on, run as root-owned files with no npm, yarn or
  corepack, and carry their own `HEALTHCHECK`; the prod and small compose files run them with `init`. CI checks the
  whole storefront workspace in one job and smoke-runs each storefront image.
- **`formatPrice` takes a locale**, and writes a currency with no symbol configured the locale's way rather than as its
  code. astro-shop formats every price through it, in the channel's language.
