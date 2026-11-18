import { afterEach, describe, expect, it, vi } from 'vitest'

const globals = globalThis as Record<string, unknown>

afterEach(() => {
  delete globals.__SKELET_DASHBOARD__
  delete globals.__SKELET_DASHBOARD_READ__
  delete globals.__SKELET_PACKAGE__
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

// The published package has no settings of its own to fall back to, and a default endpoint is where an admin's
// sign-in would go - so it starts configured or not at all
describe('the published package’s configuration', () => {
  it('refuses to load unconfigured', async () => {
    globals.__SKELET_PACKAGE__ = true
    await expect(import('./runtime-config')).rejects.toThrow(/no configuration/)
  })

  it('loads once configure() named the endpoint, with no local default for the storefront', async () => {
    globals.__SKELET_PACKAGE__ = true
    const { configure } = await import('./package/config')
    configure({ graphqlUrl: '/api/admin/x1/graphql' })
    const { runtimeConfig } = await import('./runtime-config')
    expect(runtimeConfig.graphqlUrl).toBe('/api/admin/x1/graphql')
    expect(runtimeConfig.storefrontOrigin).toBe('')
  })

  it('refuses a configure() that names no endpoint, or comes after the dashboard read its configuration', async () => {
    const { configure } = await import('./package/config')
    expect(() => configure({ graphqlUrl: '' })).toThrow(/graphqlUrl/)
    expect(() => configure({ storefrontOrigin: 'https://shop.example.com' })).toThrow(/graphqlUrl/)

    await import('./runtime-config')
    expect(() => configure({ graphqlUrl: '/api/admin/graphql' })).toThrow(/after the dashboard had loaded/)
  })
})
