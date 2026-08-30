import { ADMIN_GRAPHQL_ENDPOINT } from '@/lib/api-endpoint'
export type SurfaceId = 'admin' | 'front'

export type Surface = {
  id: SurfaceId
  label: string
  endpoint: string
  hint: string
}

// Relative paths go through Vite's /api proxy in dev and the reverse proxy in prod, so one build works on host, in the container and behind a CDN
export const SURFACES: Surface[] = [
  {
    id: 'admin',
    label: 'Admin',
    endpoint: ADMIN_GRAPHQL_ENDPOINT,
    hint: 'Back-office API - the schema this dashboard runs on',
  },
  {
    id: 'front',
    label: 'Storefront',
    endpoint: import.meta.env.VITE_FRONT_GRAPHQL_URL ?? '/api/front/graphql',
    hint: 'Public storefront API - most queries need an X-Channel-Id header',
  },
]

export function surfaceById(id: SurfaceId): Surface {
  return SURFACES.find((s) => s.id === id) ?? SURFACES[0]
}

// GraphiQL persists tabs, history and headers under fixed keys, so unprefixed the two surfaces share one set of tabs and stomp each other
export function namespacedStorage(id: SurfaceId): Storage {
  const prefix = `graphiql:${id}:`
  const keys = () => Object.keys(window.localStorage).filter((k) => k.startsWith(prefix))
  return {
    getItem: (key) => window.localStorage.getItem(prefix + key),
    setItem: (key, value) => window.localStorage.setItem(prefix + key, value),
    removeItem: (key) => window.localStorage.removeItem(prefix + key),
    clear: () => keys().forEach((k) => window.localStorage.removeItem(k)),
    key: (index) => keys()[index]?.slice(prefix.length) ?? null,
    get length() {
      return keys().length
    },
  }
}
