# Storefront browser tests

What a shopper sees in a real browser: forms that keep their input after a failed submit, errors on the right
field, the cart drawer opening, the pages rendering without browser errors. Unit and component tests (`pnpm test`)
cover the logic; these cover the wiring - server actions, the real backend, hydration, layout.

```
./skelet up                                      # the dev stack, with this storefront running
./skelet test storefront                         # every spec
./skelet test storefront e2e/account.spec.ts     # one file
./skelet test storefront -g "coupon"             # by title
```

## How it runs

`skelet test storefront` starts Microsoft's Playwright image inside the storefront container's network namespace, so
the host needs no Node and no browser, and the browser reaches the dev server as `localhost:3000`. That matters:
`next dev` only serves its `/_next` chunks to localhost and `allowedDevOrigins`, and under any other host the page
never hydrates - forms then post natively and everything looks half-working. The image tag follows the
`@playwright/test` pin in package.json, since the runner and its browsers only work at the same version.

## Watch out

- **The dev database takes the writes.** Specs sign up fresh `e2e-*@skelet.test` customers, add addresses and ask
  questions. Nothing is cleaned up.
- **Wait for hydration.** Use `open()` from `helpers.ts`, not a bare `page.goto`: a click before React attaches
  submits the form natively, which is not the path a shopper takes.
- **Scope to `main`.** The header mounts a sign-in drawer with the same field labels as the auth pages.
- **Product images fail to load** from inside the container (they point at the object store's host port). Specs
  ignore them.
- **Catalogue assumptions:** `/casual-t-shirt` is the dev catalogue's simple, in-stock product, and US is in the
  countries reference data.
