import type { CodegenConfig } from '@graphql-codegen/cli'
import { shared, SIGNALS_DOCUMENTS } from './codegen.shared'

// Uses a non-VITE_ prefix on purpose: anything starting with VITE_ is inlined into the browser bundle by Vite, and pointing the bundle at
// the in-network Docker hostname (http://app:8080/…) breaks Apollo in the dev container. Codegen-only env stays server-side
const ENDPOINT = process.env.GRAPHQL_CODEGEN_URL ?? 'http://localhost:8080/api/admin/graphql'

const config: CodegenConfig = {
  overwrite: true,
  schema: ENDPOINT,
  // Everything except the Signals surface, which codegen.signals.ts validates against its own schema
  documents: ['src/**/*.graphql', `!${SIGNALS_DOCUMENTS}`],
  config: shared,
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
