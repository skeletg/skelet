# Dashboard end-to-end tests

Pointer gestures and cross-device state - dragging tabs between panes, splitting the workspace,
workspaces roaming through the preferences store. None of it is reachable from jsdom, so it runs in
a real browser against a real stack.

```
./skelet up          # dashboard on :5173, admin API on :8080, dev admin seeded
npm run test:e2e
npm run test:e2e -- --headed --project=chromium tabs.spec.ts
```

`npm test` (the unit suite) does NOT run these - CI has no backend, and these need one.

Overrides: `E2E_DASHBOARD_URL`, `E2E_API_URL`, `E2E_ADMIN_EMAIL`, `E2E_ADMIN_PASSWORD`. When the
root `.env` sets `ADMIN_API_SLUG`, export it too - the API helpers find the admin endpoint by it.

`forms.spec.ts` runs the staged-save forms against the real API: it creates its own records,
edits them through the UI and reads them back, then deletes them. What it can't judge by eye is
in `FORMS-QA.md`.

`smoke.spec.ts` opens every page in `src/nav/routes.tsx` and fails the page's step on a GraphQL
error, an uncaught exception, a `console.error`, the route error boundary or the 404 page. Fixed
paths are opened as they are. For each `:id` route, `detailResolvers` finds a real record: it seeds
one when the API can make it cheaply, or takes the first the dev data has. A route with neither is
skipped and named in the test's annotations. A new parameterised route with no resolver fails, so
it gets one. It reads the paths from the source, so new pages are covered without touching the spec. It runs as
the e2e admin, so a resolver that refuses a role the page is open to shows up here - a superuser
bypasses those checks and hides it. Signals must be up, or every suite page fails with
`SIGNALS_UNAVAILABLE`.

The form specs each check one part of the staged-save forms against the real API. They share
`fixtures.ts` and delete what they create when the file finishes:

- `record-pages.spec.ts`: every edit page. It seeds a record with the optional fields set, changes
  one field through the page, and reads the record back through the page's own query (picked out
  of `src/**/*.graphql`). Anything else that moved fails the case. That covers a null saved as
  `""`, a date shifted by the browser's timezone, and a list the update dropped.
- `record-create.spec.ts`: the create pages. It covers what the server stored, the `new` draft
  cleared after the create, a server error shown under its field, and a stored draft restored
  after a reload.
- `drafts.spec.ts`: the stored draft's life. It covers a clash with a change made elsewhere, two
  sections each offering their own draft, editing instead of deciding, closing the workspace tab,
  a sibling tab on the same record, and logging out (in its own sign-in).
- `dialogs.spec.ts`: the dialog forms, including edit, address and shipment on a placed order.
- `store-scope.spec.ts`: opens a store of its own, with a throwaway admin whose role reaches only
  that store. It checks the scope picker offers that store alone, and that the Default Store's
  customers and orders stay out of reach, in the lists and by their direct links.
- `readonly.spec.ts`: a throwaway admin on a view-only role. It gets no Save, disabled inputs and
  no drafts, and can still change its own password.
- `translations.spec.ts`: runs every query the dashboard sends over records seeded in two
  locales. It fails any response that lists fewer translation rows than the record has, because
  Apollo would let that short list replace the full one the form's baseline is built from.

Some cases skip, and say why, when the e2e admin lacks a permission they need: tax, shipping and
shipping boxes need `settings.manage`. `dialogs.spec.ts` leaves one cancelled order per run behind,
since orders can't be deleted; its email starts with `e2e-forms-`.

Every run needs `ADMIN_API_SLUG` and, unless the dev admin still has the default,
`E2E_ADMIN_PASSWORD`.

## First run

`npx playwright install chromium` fetches the browser. On a bare Linux host also
`npx playwright install-deps chromium` (needs root) - without it Chromium cannot find libnss3 and
exits before the first test. If you can't install system packages, extract them and point
`LD_LIBRARY_PATH` at them before invoking the runner.

## Watch out

Import `test` from `./helpers`, not from `@playwright/test`. The suite signs in once, and an admin
refresh token is single-use, so a run longer than the 15-minute access token can't replay the saved
sign-in: the first page to refresh would burn it and every later test would land on the sign-in
page. The helpers' `test` starts each browser from the live session instead, refreshing it between
tests and writing it back to `e2e/.auth/admin.json`.

Workspaces roam, so they now persist **server-side per admin** between runs. Every spec calls
`resetWorkspaces()` before it touches the UI - without it a run inherits the last one's tabs and
assertions drift. That is also why `workers: 1`: the specs share one admin account.
