import { defineConfig } from "vitest/config";

// The client components that need Next, rendered in jsdom with Testing Library; their server actions are mocked
export default defineConfig({
  test: {
    include: ["**/*.test.tsx"],
    exclude: ["node_modules/**"],
    environment: "jsdom",
    setupFiles: ["./vitest.setup.ts"],
  },
});
