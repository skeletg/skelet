import type { ConfigSlot, DashboardConfig } from '../runtime-config'

// @skelet/dashboard/config - call it before any of the dashboard loads, since it reads these as its modules do. Import
// the dashboard dynamically after it, as the template's main.ts does: a bundler runs a statically imported chunk
// before the code importing it, whatever the order of the imports. Kept apart from everything else so importing it
// loads none of the dashboard. Called too late, or without the endpoint, it throws - the alternative is a dashboard
// quietly talking to a default
export function configure(config: DashboardConfig) {
  const slot = globalThis as ConfigSlot
  if (slot.__SKELET_DASHBOARD_READ__) {
    throw new Error(
      '@skelet/dashboard: configure() ran after the dashboard had loaded and read its configuration. Import your config ' +
        'first and the dashboard dynamically after it - a static import of the dashboard runs before the code around it',
    )
  }
  if (!config.graphqlUrl) {
    throw new Error('@skelet/dashboard: configure() needs graphqlUrl, the admin GraphQL endpoint - is VITE_GRAPHQL_URL set in .env?')
  }
  slot.__SKELET_DASHBOARD__ = config
}

export type { DashboardConfig }
