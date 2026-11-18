// What this deployment's dashboard talks to. Built from this repo, it's the app's .env through Vite as ever; the
// published @skelet/dashboard was compiled before any project's .env existed, so a project's build hands the values
// over through @skelet/dashboard/config, which has to run before any of the dashboard's modules read them here
export type DashboardConfig = {
  graphqlUrl?: string // the admin GraphQL endpoint, relative to the page or absolute - the package can't do without it
  frontGraphqlUrl?: string // the storefront's, for the playground
  storefrontOrigin?: string // where the CMS and blog previews frame the storefront
  sentryDsn?: string // error monitoring, off without one
  environment?: string // what Sentry calls the deployment
  basePath?: string // where the provider sign-in comes back to, ending in / - the dashboard itself is served from the root
}

// SYNC: src/lib/package/config.ts writes the first and reads the second
export type ConfigSlot = { __SKELET_DASHBOARD__?: DashboardConfig; __SKELET_DASHBOARD_READ__?: boolean }

// Set by vite.lib.config.ts and nowhere else. There the configuration comes from configure() alone: the package's own
// build has no .env to read, and a default would send an admin's sign-in to whatever origin it happened to name
declare const __SKELET_PACKAGE__: boolean | undefined
const packaged = typeof __SKELET_PACKAGE__ !== 'undefined' && __SKELET_PACKAGE__ === true

const slot = globalThis as ConfigSlot
const injected = slot.__SKELET_DASHBOARD__ ?? {}
if (packaged && !injected.graphqlUrl) {
  throw new Error(
    "@skelet/dashboard has no configuration. Call configure({ graphqlUrl, ... }) from '@skelet/dashboard/config' before " +
      'the dashboard loads, and import the dashboard dynamically after it - see the template\'s src/main.ts',
  )
}
slot.__SKELET_DASHBOARD_READ__ = true

export const runtimeConfig = {
  graphqlUrl: injected.graphqlUrl ?? import.meta.env.VITE_GRAPHQL_URL ?? '/api/admin/graphql',
  frontGraphqlUrl: injected.frontGraphqlUrl ?? (import.meta.env.VITE_FRONT_GRAPHQL_URL as string | undefined),
  storefrontOrigin: injected.storefrontOrigin ?? (import.meta.env.VITE_STOREFRONT_ORIGIN as string | undefined) ?? (packaged ? '' : 'http://localhost:3000'),
  sentryDsn: injected.sentryDsn ?? (import.meta.env.VITE_SENTRY_DSN as string | undefined),
  environment: injected.environment ?? import.meta.env.MODE,
  basePath: injected.basePath ?? import.meta.env.BASE_URL,
}
