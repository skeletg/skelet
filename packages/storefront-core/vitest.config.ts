import { defineConfig } from "vitest/config";

// Plain modules in node - nothing here renders, and nothing needs a backend: a test answers request() with
// useTestRequest from ./testing, and fetch with a mock
export default defineConfig({
  test: { include: ["**/*.test.ts"], exclude: ["node_modules/**"], environment: "node" },
});
