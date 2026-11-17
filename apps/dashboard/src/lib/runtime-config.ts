// What this deployment's dashboard talks to. Built from this repo, it's the app's .env through Vite as ever; the
// published @skelet/dashboard was compiled before any project's .env existed, so a project's build hands the values
// over through @skelet/dashboard/config, which runs before any of the dashboard's modules read them here
export type DashboardConfig = {
  graphqlUrl?: string // the admin GraphQL endpoint, relative to the page or absolute
  frontGraphqlUrl?: string // the storefront's, for the playground
  storefrontOrigin?: string // where the CMS and blog previews frame the storefront
  sentryDsn?: string // error monitoring, off without one
  environment?: string // what Sentry calls the deployment
  basePath?: string // the path the dashboard is served under, ending in /
}

// SYNC: src/lib/package/config.ts writes it
const injected = ((globalThis as Record<string, unknown>).__SKELET_DASHBOARD__ ?? {}) as DashboardConfig

export const runtimeConfig = {
  graphqlUrl: injected.graphqlUrl ?? import.meta.env.VITE_GRAPHQL_URL ?? '/api/admin/graphql',
  frontGraphqlUrl: injected.frontGraphqlUrl ?? (import.meta.env.VITE_FRONT_GRAPHQL_URL as string | undefined),
  storefrontOrigin: injected.storefrontOrigin ?? (import.meta.env.VITE_STOREFRONT_ORIGIN as string | undefined) ?? 'http://localhost:3000',
  sentryDsn: injected.sentryDsn ?? (import.meta.env.VITE_SENTRY_DSN as string | undefined),
  environment: injected.environment ?? import.meta.env.MODE,
  basePath: injected.basePath ?? import.meta.env.BASE_URL,
}
