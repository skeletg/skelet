import type { CodegenConfig } from "@graphql-codegen/cli";

// FRONT surface, not admin. Defaults to the host port so codegen works outside Docker - compose
// sets the in-network app:8080 for runs inside the container
const ENDPOINT = process.env.GRAPHQL_CODEGEN_URL ?? "http://localhost:8080/api/front/graphql";

const config: CodegenConfig = {
  overwrite: true,
  schema: ENDPOINT,
  documents: ["src/**/*.graphql"],
  config: {
    useTypeImports: true,
    enumsAsTypes: true,
    scalars: { Time: "string", JSON: "unknown" },
  },
  // The schema's types are the core's - generated once there, so this shop's operations and the core's agree
  generates: {
    "src/": {
      preset: "near-operation-file",
      presetConfig: { extension: ".gql.ts", baseTypesPath: "~@skelet/storefront-core/graphql/schema.types" },
      plugins: ["typescript-operations", "typed-document-node"],
    },
  },
};

export default config;
