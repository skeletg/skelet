import eslintPluginAstro from "eslint-plugin-astro";
import tseslint from "typescript-eslint";

export default [
  { ignores: ["dist/", ".astro/", "node_modules/", "src/**/*.gql.ts", "src/graphql/", "test-results/", "playwright-report/"] },
  ...tseslint.configs.recommended,
  ...eslintPluginAstro.configs.recommended,
];
