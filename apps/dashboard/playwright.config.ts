import { defineConfig, devices } from '@playwright/test'

// Declared here rather than imported from ./e2e/helpers - pulling a spec helper into the config's
// own module graph makes Playwright reject the test.beforeEach calls in files that share it
const AUTH_FILE = 'e2e/.auth/admin.json'

// Browser-level coverage for the workspace shell - tab dragging, split panes and roaming workspaces
// are all pointer gestures and cross-device state, none of which jsdom can speak to
//
// Needs the dev stack up (`./skelet up`): the dashboard on 5173 and the admin API on 8080, seeded
// with the dev admin. Deliberately NOT part of `npm test`, which CI runs without a backend
export default defineConfig({
  testDir: './e2e',
  // the suite drives one shared admin account, and workspaces roam per admin - parallel files
  // would fight over the same preference row
  workers: 1,
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? 'line' : 'list',
  timeout: 90_000,
  expect: { timeout: 15_000 },
  use: {
    baseURL: process.env.E2E_DASHBOARD_URL ?? 'http://localhost:5173',
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    video: 'off',
  },
  projects: [
    { name: 'setup', testMatch: /auth\.setup\.ts/ },
    {
      name: 'chromium',
      dependencies: ['setup'],
      use: { ...devices['Desktop Chrome'], viewport: { width: 1600, height: 950 }, storageState: AUTH_FILE },
    },
  ],
})
