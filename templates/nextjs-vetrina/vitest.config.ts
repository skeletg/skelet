import { fileURLToPath } from "node:url";
import { defineConfig } from "vitest/config";

// Two projects. "unit" is everything in node - the pure helpers, route handlers, and components' plain .ts helpers (the
// slot registry is plain data with no React in it, and it decides what a plugin can put on a page, so it earns a test). "components" renders client
// components in jsdom with Testing Library; server actions are mocked per test, so nothing there needs a backend
export default defineConfig({
  resolve: {
    // Mirror tsconfig's "@/*" -> "./*" so tests import exactly like app code
    alias: { "@": fileURLToPath(new URL(".", import.meta.url)) },
  },
  test: {
    projects: [
      {
        extends: true,
        test: {
          name: "unit",
          include: ["app/**/*.test.ts"],
          environment: "node",
        },
      },
      {
        extends: true,
        test: {
          name: "components",
          include: ["app/**/*.test.tsx"],
          environment: "jsdom",
          setupFiles: ["./vitest.setup.ts"],
        },
      },
    ],
  },
});
