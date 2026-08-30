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
  // Override default ignores of eslint-config-next.
  globalIgnores([
    // Default ignores of eslint-config-next:
    ".next/**",
    "out/**",
    "build/**",
    "next-env.d.ts",
    // bin/codegen output - regenerated wholesale, so a finding here has nothing to fix by hand
    "app/**/*.gql.ts",
    "app/graphql/schema.types.ts",
  ]),
]);

export default eslintConfig;
