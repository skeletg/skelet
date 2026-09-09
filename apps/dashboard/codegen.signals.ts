import type { CodegenConfig } from '@graphql-codegen/cli'
import { shared, SIGNALS_DOCUMENTS } from './codegen.shared'

// Reads the .graphqls files rather than the endpoint, since its schema has no plugin merging and needing a second service up to generate types is worse
const SIGNALS_SCHEMA = '../backend/graph/schema/signals/*.graphqls'

// Its own config file rather than a second project inside codegen.ts, because a root-level documents glob overrides an output-level one and a single config silently generated both surfaces from the admin schema
const config: CodegenConfig = {
  overwrite: true,
  schema: SIGNALS_SCHEMA,
  documents: [SIGNALS_DOCUMENTS],
  config: shared,
  generates: {
    'src/graphql/signals.types.ts': {
      plugins: ['typescript'],
    },
    'src/': {
      preset: 'near-operation-file',
      presetConfig: {
        extension: '.gql.ts',
        baseTypesPath: 'graphql/signals.types.ts',
      },
      plugins: ['typescript-operations', 'typed-document-node'],
    },
  },
  hooks: {
    afterAllFileWrite: [
      'node scripts/strip-unused-types-import.mjs',
      // Writes the operation names the Apollo split link routes to the Signals endpoint
      'node scripts/signals-operations.mjs',
    ],
  },
}

export default config
