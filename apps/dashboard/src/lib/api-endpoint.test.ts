import { afterEach, describe, expect, it, vi } from 'vitest'

async function endpoints(graphqlUrl: string | undefined) {
  vi.resetModules()
  if (graphqlUrl === undefined) vi.unstubAllEnvs()
  else vi.stubEnv('VITE_GRAPHQL_URL', graphqlUrl)
  return import('./api-endpoint')
}

afterEach(() => vi.unstubAllEnvs())

// Derived from the GraphQL endpoint, slug and all, an SVG upload got a 404 whenever a slug was set
describe('admin HTTP endpoints', () => {
  it.each([
    [undefined, '/api/admin'],
    ['/api/admin/graphql', '/api/admin'],
    ['/api/admin/admin-65da2d161cb5786c/graphql', '/api/admin'],
    ['https://api.shop.test/api/admin/admin-65da2d161cb5786c/graphql', 'https://api.shop.test/api/admin'],
  ])('with the GraphQL endpoint at %s they sit under %s', async (graphqlUrl, base) => {
    const m = await endpoints(graphqlUrl)
    expect(m.ADMIN_MEDIA_SVG_ENDPOINT).toBe(`${base}/media/svg`)
    expect(m.ADMIN_MEDIA_FILE_ENDPOINT).toBe(`${base}/media/file`)
  })
})
