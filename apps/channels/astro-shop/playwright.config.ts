import { defineConfig, devices } from "@playwright/test";

// Browser coverage against the real backend: browse, cart, checkout, SEO and motion, with and without JavaScript.
// Runs through `./skelet test storefront astro-shop`, which starts Microsoft's Playwright image in the storefront
// container's network. Not part of `pnpm test` (the unit suite), which needs no backend. Places real orders in the
// dev database, as vetrina-demo's suite does
export default defineConfig({
  testDir: "./e2e",
  // Every spec gets its own cart through its own cookies, but they share one dev server compiling routes on first hit
  workers: 2,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? "line" : "list",
  timeout: 60_000,
  expect: { timeout: 10_000 },
  use: {
    baseURL: process.env.E2E_BASE_URL ?? "http://localhost:3000",
    trace: "retain-on-failure",
    video: "off",
  },
  projects: [
    { name: "chromium", use: { ...devices["Desktop Chrome"] } },
    // The whole money path is plain forms - prove it works with scripts off
    { name: "no-js", testMatch: /(cart|checkout)\.spec\.ts/, use: { ...devices["Desktop Chrome"], javaScriptEnabled: false } },
    { name: "mobile", testMatch: /shop\.spec\.ts/, use: { ...devices["Pixel 7"] } },
  ],
});
