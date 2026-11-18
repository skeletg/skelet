// The packages the dashboard shares with the project and its plugins rather than owning. One list: they're the
// published package's peers, what its Vite preset dedupes, and what the template and a plugin's own package.json name
// - a second copy of any of them is a second React, Apollo cache, router, translation context or toast store that the
// dashboard's own never hears from
export const shared = ['react', 'react-dom', 'react-router-dom', '@apollo/client', '@lingui/core', '@lingui/react', 'sonner']

// The playground's Monaco worker registration is a bare `?worker` chain the dep optimizer can't pre-bundle - optimizing
// it serves a 504 and the whole route fails to load
export const unbundled = ['graphiql/setup-workers/vite', '@graphiql/react/setup-workers/vite']

// The two CommonJS packages inside the playground's GraphQL worker - served raw, `import x from` finds no default
// export, the worker dies at load and Monaco runs the language service on the main thread, freezing the page
export const workerCommonJS = [
  'graphiql > @graphiql/react > monaco-graphql > graphql-language-service > nullthrows',
  'graphiql > @graphiql/react > monaco-graphql > picomatch-browser',
]
