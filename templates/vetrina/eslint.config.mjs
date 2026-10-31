import { defineConfig, globalIgnores } from "eslint/config";
import nextVitals from "eslint-config-next/core-web-vitals";
import nextTs from "eslint-config-next/typescript";

const eslintConfig = defineConfig([
  ...nextVitals,
  ...nextTs,
  // eslint-plugin-react 7.37.5 (even the latest) still calls context.getFilename() - removed in
  // ESLint 10 - inside its react-version DETECTION path, which crashed every lint run. An explicit
  // version keeps that path cold; keep it in step with package.json's react on major bumps
  {
    settings: { react: { version: "19.2.8" } },
  },
  // Shared code must not reach into a route folder - anything two routes need lives in
  // app/components or app/lib, so moving or deleting a route can't break an unrelated page
  {
    files: ["app/components/**", "app/lib/**", "app/plugins/**"],
    rules: {
      "no-restricted-imports": [
        "error",
        {
          patterns: [
            {
              group: ["@/app/\\[channel\\]/**"],
              message: "Shared code can't import from a route folder - move the module to app/components or app/lib.",
            },
          ],
        },
      ],
    },
  },
  // Override default ignores of eslint-config-next.
  globalIgnores([
    // Default ignores of eslint-config-next:
    ".next/**",
    "out/**",
    "build/**",
    "next-env.d.ts",
  ]),
]);

export default eslintConfig;
