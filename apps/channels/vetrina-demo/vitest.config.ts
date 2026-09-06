import { fileURLToPath } from "node:url";
import { defineConfig } from "vitest/config";

// Pure helpers only - the app itself is covered by typecheck + build. app/plugins is in scope for the
// same reason app/lib is: the slot registry is plain data with no React in it, and it decides what a
// plugin can put on a page, so it earns a test
export default defineConfig({
  resolve: {
    // Mirror tsconfig's "@/*" -> "./*" so tests import exactly like app code
    alias: { "@": fileURLToPath(new URL(".", import.meta.url)) },
  },
  test: {
    include: ["app/lib/**/*.test.ts", "app/plugins/**/*.test.ts"],
    environment: "node",
  },
});
