import { configure } from '@skelet/dashboard/config'

// What this deployment's dashboard talks to, from .env at build time - see .env.example
configure({
  graphqlUrl: import.meta.env.VITE_GRAPHQL_URL,
  frontGraphqlUrl: import.meta.env.VITE_FRONT_GRAPHQL_URL,
  storefrontOrigin: import.meta.env.VITE_STOREFRONT_ORIGIN,
  sentryDsn: import.meta.env.VITE_SENTRY_DSN,
  environment: import.meta.env.MODE,
  basePath: import.meta.env.BASE_URL,
})
