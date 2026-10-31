import type { CodegenConfig } from "@graphql-codegen/cli";

// FRONT surface, not admin. Defaults to the host port so codegen works outside Docker - compose
// sets the in-network app:8080 for runs inside the container
const ENDPOINT =
  process.env.GRAPHQL_CODEGEN_URL ?? "http://localhost:8080/api/front/graphql";

const config: CodegenConfig = {
  overwrite: true,
  schema: ENDPOINT,
  documents: ["app/**/*.graphql"],
  config: {
    useTypeImports: true,
    enumsAsTypes: true,
    // Time is an ISO string everywhere it's read; JSON is Lexical/arbitrary, so callers
    // parse it themselves
    scalars: {
      Time: "string",
      JSON: "unknown",
    },
  },
  generates: {
    "app/graphql/schema.types.ts": {
      plugins: ["typescript"],
    },
    "app/": {
      preset: "near-operation-file",
      presetConfig: {
        extension: ".gql.ts",
        baseTypesPath: "graphql/schema.types.ts",
      },
      plugins: ["typescript-operations", "typed-document-node"],
    },
  },
};

export default config;
