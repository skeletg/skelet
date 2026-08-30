import type { CodegenConfig } from '@graphql-codegen/cli'

// Uses a non-VITE_ prefix on purpose: anything starting with VITE_ is inlined into the browser bundle by Vite, and pointing the bundle at
// the in-network Docker hostname (http://app:8080/…) breaks Apollo in the dev container. Codegen-only env stays server-side
const ENDPOINT = process.env.GRAPHQL_CODEGEN_URL ?? 'http://localhost:8080/api/admin/graphql'

const config: CodegenConfig = {
  overwrite: true,
  schema: ENDPOINT,
  documents: ['src/**/*.graphql'],
  config: {
    useTypeImports: true,
    enumsAsTypes: true,
    // Int64 (aggregate money in cents) marshals as a JSON number; JS handles
    // values well under 2^53, so number is safe and keeps arithmetic clean.
    // Time is gqlgen's RFC3339 string on the wire. JSON stays schemaless
    // (rich-text bodies, block documents) - unknown forces consumers to
    // narrow instead of silently opting out of type checking.
    scalars: {
      Int64: 'number',
      Time: 'string',
      JSON: 'unknown',
    },
  },
  generates: {
    // Base schema types - referenced by every per-operation file below.
    'src/graphql/schema.types.ts': {
      plugins: ['typescript'],
    },
    // One .gql.ts next to each .graphql source file.
    'src/': {
      preset: 'near-operation-file',
      presetConfig: {
        extension: '.gql.ts',
        baseTypesPath: 'graphql/schema.types.ts',
      },
      plugins: ['typescript-operations', 'typed-document-node'],
    },
  },
  // See the script - the preset's Types import is unused in operations that touch no enum,
  // input or custom scalar, and noUnusedLocals rejects those files
  hooks: {
    afterAllFileWrite: ['node scripts/strip-unused-types-import.mjs'],
  },
}

export default config
