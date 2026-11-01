/// <reference types="vitest/config" />
import { getViteConfig } from "astro/config";

// Astro's Vite config, so page tests can compile .astro files and render them through the Container API.
// The unit suite needs no backend - e2e/ is Playwright's, run by `./skelet test storefront astro-shop`
export default getViteConfig({
  test: {
    include: ["src/**/*.test.{ts,tsx}"],
    environment: "node",
  },
});
