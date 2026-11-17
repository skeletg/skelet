import { afterEach, describe, expect, it, vi } from 'vitest'

afterEach(() => {
  delete (globalThis as Record<string, unknown>).__SKELET_DASHBOARD__
  vi.resetModules()
})

describe('the dashboard configuration', () => {
  it('takes what configure() set before the dashboard loaded', async () => {
    const { configure } = await import('./package/config')
    configure({ graphqlUrl: 'https://api.example.com/api/admin/x1/graphql', storefrontOrigin: 'https://shop.example.com/' })
    const { ADMIN_GRAPHQL_ENDPOINT, API_ORIGIN } = await import('./api-endpoint')
    const { STOREFRONT_ORIGIN } = await import('./app-origins')
    expect(ADMIN_GRAPHQL_ENDPOINT).toBe('https://api.example.com/api/admin/x1/graphql')
    expect(API_ORIGIN).toBe('https://api.example.com')
    expect(STOREFRONT_ORIGIN).toBe('https://shop.example.com')
  })

  it('falls back to the app’s own build settings without one', async () => {
    const { runtimeConfig } = await import('./runtime-config')
    expect(runtimeConfig.graphqlUrl).toBeTruthy()
    expect(runtimeConfig.basePath).toBe('/')
  })
})
