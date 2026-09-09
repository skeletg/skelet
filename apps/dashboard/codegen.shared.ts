// Shared by the two codegen configs - one per GraphQL surface the dashboard talks to

// Which surface serves an operation is declared by its filename, so a misfiled one fails codegen instead of 404ing at runtime
export const SIGNALS_DOCUMENTS = 'src/**/*.signals.graphql'

// Int64 (money in cents) marshals as a JSON number, Time is gqlgen's RFC3339 string, and JSON stays unknown so consumers narrow instead of opting out of type checking
export const shared = {
  useTypeImports: true,
  enumsAsTypes: true,
  scalars: {
    Int64: 'number',
    Time: 'string',
    JSON: 'unknown',
  },
}
