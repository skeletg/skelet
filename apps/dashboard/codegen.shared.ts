// Shared by the codegen config

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
