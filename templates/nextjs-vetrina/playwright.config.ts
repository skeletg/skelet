import { defineConfig, devices } from "@playwright/test";

// Browser coverage for the storefront's forms - what a shopper sees when a submit fails or succeeds, which needs the
// real server actions, the real backend and a real browser's form reset. Runs through `skelet test storefront`,
// which starts Microsoft's Playwright image on the compose network; see e2e/README.md
//
// Not part of `pnpm test` (the unit suite), which runs without a backend
export default defineConfig({
  testDir: "./e2e",
  // Specs sign up their own customers, so files don't share state - but they do share one dev server that compiles
  // routes on first hit, and two workers racing that compile is where flakes come from
  workers: 2,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? "line" : "list",
  // next dev compiles a route on its first request - the first visit to each page can take seconds
  timeout: 60_000,
  expect: { timeout: 10_000 },
  use: {
    baseURL: process.env.E2E_BASE_URL ?? "http://localhost:3000",
    trace: "retain-on-failure",
    video: "off",
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
});
