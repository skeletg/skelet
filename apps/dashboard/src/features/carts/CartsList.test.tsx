import { screen, waitFor } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { globalScope, meMock, renderPage } from '@/test/page-harness'
import { CartsList } from './CartsList'
import { CartsDocument } from './Carts.gql'

// What this pins: shopper carts page on the server. The URL's page and size become the query's
// offset and limit, and the server's totalCount drives the pager. The list used to load the first
// 100 carts and page them in the browser, so cart 101 onwards could never be reached

const channelScope = { ...globalScope, scope: { mode: 'CHANNEL' as const, channelId: 'c1', locale: '' }, effectiveChannelId: 'c1', isChannel: true, isGlobal: false }

const cart = {
  __typename: 'CartListItem',
  id: 'cart-21',
  customerId: null,
  customerEmail: 'shopper@example.com',
  customerName: 'Shopper Twentyone',
  status: 'active',
  kind: 'SHOPPER',
  email: 'shopper@example.com',
  offerState: null,
  offerExpiresAt: null,
  orderId: null,
  itemCount: 2,
  subtotal: 1500,
  currencyCode: 'EUR',
  updatedAt: '2026-10-01T10:00:00Z',
}

describe('CartsList', () => {
  it('asks the server for the page in the URL and pages by its total', async () => {
    const seen: Record<string, unknown>[] = []
    const carts = {
      request: { query: CartsDocument, variables: (v: Record<string, unknown>) => (seen.push(v), true) },
      result: { data: { carts: { __typename: 'CartList', totalCount: 45, items: [cart] } } },
      maxUsageCount: 5,
    }
    renderPage({ path: '/carts?page=3&size=10', routes: { '/carts': <CartsList /> }, mocks: [meMock(), carts], scope: channelScope, adminId: 'admin-1' })

    expect(await screen.findByText('Shopper Twentyone')).toBeDefined()
    expect(seen[0]).toMatchObject({ channelId: 'c1', kind: 'SHOPPER', status: 'active', limit: 10, offset: 20, search: null })
    // The server's total, though only one row came back
    await waitFor(() => expect(screen.getByText('45 rows')).toBeDefined())
  })
})
