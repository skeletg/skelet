import { defineConfig } from "vitest/config";

// Two projects, as in the storefronts: the plain modules in node, and the client providers rendered in jsdom with
// Testing Library. Server actions are mocked per test, so nothing here needs a backend
export default defineConfig({
  test: {
    projects: [
      {
        extends: true,
        test: { name: "unit", include: ["**/*.test.ts"], exclude: ["node_modules/**"], environment: "node" },
      },
      {
        extends: true,
        test: {
          name: "components",
          include: ["**/*.test.tsx"],
          exclude: ["node_modules/**"],
          environment: "jsdom",
          setupFiles: ["./vitest.setup.ts"],
        },
      },
    ],
  },
});
