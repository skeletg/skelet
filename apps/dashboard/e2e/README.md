# Dashboard end-to-end tests

Pointer gestures and cross-device state - dragging tabs between panes, splitting the workspace,
workspaces roaming through the preferences store. None of it is reachable from jsdom, so it runs in
a real browser against a real stack.

```
./skelet up          # dashboard on :5173, admin API on :8080, dev admin seeded
pnpm run test:e2e
pnpm run test:e2e --headed --project=chromium tabs.spec.ts
```

`pnpm test` (the unit suite) does NOT run these - CI has no backend, and these need one.

Overrides: `E2E_DASHBOARD_URL`, `E2E_API_URL`, `E2E_ADMIN_EMAIL`, `E2E_ADMIN_PASSWORD`.

## First run

`pnpm exec playwright install chromium` fetches the browser. On a bare Linux host also
`pnpm exec playwright install-deps chromium` (needs root) - without it Chromium cannot find libnss3 and
exits before the first test. If you can't install system packages, extract them and point
`LD_LIBRARY_PATH` at them before invoking the runner.

## Watch out

Workspaces roam, so they now persist **server-side per admin** between runs. Every spec calls
`resetWorkspaces()` before it touches the UI - without it a run inherits the last one's tabs and
assertions drift. That is also why `workers: 1`: the specs share one admin account.
