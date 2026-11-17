import type { DashboardConfig } from '../runtime-config'

// @skelet/dashboard/config - call it before any of the dashboard loads, since it reads these as its modules do. Import
// the dashboard and the plugins dynamically after it, as the template's main.ts does: a bundler runs a statically
// imported chunk before the code importing it, whatever the order of the imports. Kept apart from everything else so
// importing it loads none of the dashboard
export function configure(config: DashboardConfig) {
  // SYNC: src/lib/runtime-config.ts reads it
  ;(globalThis as Record<string, unknown>).__SKELET_DASHBOARD__ = config
}

export type { DashboardConfig }
