import { fileURLToPath } from "node:url";
import { defineConfig } from "vitest/config";

// Pure lib helpers only - the app itself is covered by typecheck + build
export default defineConfig({
  resolve: {
    // Mirror tsconfig's "@/*" -> "./*" so tests import exactly like app code
    alias: { "@": fileURLToPath(new URL(".", import.meta.url)) },
  },
  test: {
    include: ["app/lib/**/*.test.ts"],
    environment: "node",
  },
});
