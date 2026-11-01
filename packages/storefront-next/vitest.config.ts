import { defineConfig } from "vitest/config";

// The proxy in node; the client components that need Next rendered in jsdom with Testing Library, their server actions
// mocked
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
