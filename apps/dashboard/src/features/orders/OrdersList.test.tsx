import { fireEvent, screen, waitFor } from '@testing-library/react'
import type { MockLink } from '@apollo/client/testing'
import { beforeAll, describe, expect, it, vi } from 'vitest'
import { meMock, renderPage } from '@/test/page-harness'
import { PluginListFiltersDocument } from '@/plugins/ListFilters.gql'
import { OrdersList } from './OrdersList'
import { OrderStagesDocument, OrdersPagedDocument } from './Orders.gql'

// What these pin: the header only offers what the admin can open - New order needs orders.update and
// Export CSV the data hub's data.view, the same permissions their routes check. A viewer used to get
// both buttons, each leading to an access-restricted page

const empty = {
  request: { query: OrdersPagedDocument, variables: () => true },
  result: { data: { ordersPaged: { __typename: 'OrderPage', totalCount: 0, items: [] } } },
  maxUsageCount: 5,
}

const stagesMock = (orderStages: { key: string; label: string; within: string[] }[]) => ({
  request: { query: OrderStagesDocument, variables: () => true },
  result: { data: { orderStages: orderStages.map((st) => ({ __typename: 'OrderStage', ...st })) } },
  maxUsageCount: 5,
})

const pluginFiltersMock = (pluginListFilters: { key: string; label: string; options: { value: string; label: string }[] }[]) => ({
  request: { query: PluginListFiltersDocument, variables: { list: 'ORDERS' } },
  result: {
    data: {
      pluginListFilters: pluginListFilters.map((f) => ({
        __typename: 'PluginListFilter',
        plugin: f.key.split(':')[0],
        ...f,
        options: f.options.map((o) => ({ __typename: 'PluginListFilterOption', ...o })),
      })),
    },
  },
  maxUsageCount: 5,
})

function render(
  me: ReturnType<typeof meMock>,
  mocks: MockLink.MockedResponse[] = [empty, stagesMock([]), pluginFiltersMock([])],
) {
  // The table keeps its column and density choices as the admin's preferences
  renderPage({ path: '/orders', routes: { '/orders': <OrdersList /> }, mocks: [me, ...mocks], adminId: 'admin-1' })
}

describe('OrdersList', () => {
  beforeAll(() => {
    // cmdk scrolls the active filter option into view; jsdom has no scrolling API
    HTMLElement.prototype.scrollIntoView = vi.fn()
  })

  it('leaves New order and Export CSV out for an admin who can only view orders', async () => {
    render(meMock({ superuser: false, permissions: ['orders.view'] }))
    expect(await screen.findByText('No orders yet.')).toBeDefined()
    expect(screen.queryByRole('link', { name: /New order/ })).toBeNull()
    expect(screen.queryByRole('link', { name: /Export CSV/ })).toBeNull()
  })

  it('offers both to an admin who can create orders and export data', async () => {
    render(meMock({ superuser: false, permissions: ['orders.view', 'orders.update', 'data.view'] }))
    await waitFor(() => expect(screen.getByRole('link', { name: /New order/ })).toBeDefined())
    expect(screen.getByRole('link', { name: /Export CSV/ })).toBeDefined()
  })

  it('shows the stage of each order and filters by it once a plugin offers stages', async () => {
    const staged = {
      ...empty,
      result: { data: { ordersPaged: { __typename: 'OrderPage', totalCount: 1, items: [{
        __typename: 'Order', id: 'o1', number: '1001', status: 'PROCESSING', paymentStatus: 'UNPAID', total: 1000, currencyCode: 'EUR',
        createdAt: '2026-01-01T00:00:00Z', stage: { __typename: 'OrderStage', key: 'awaiting_stock', label: 'Awaiting stock' },
        shippingAddress: { __typename: 'OrderAddress', firstName: 'Ada', lastName: 'Buyer' },
      }] } } },
    }
    render(meMock({ superuser: false, permissions: ['orders.view'] }), [staged, stagesMock([{ key: 'awaiting_stock', label: 'Awaiting stock', within: ['PROCESSING'] }]), pluginFiltersMock([])])
    expect(await screen.findByText('Awaiting stock')).toBeDefined()
    expect(screen.getByRole('button', { name: /Stage/ })).toBeDefined()
  })

  it('leaves the stage column and filter out when no plugin offers one', async () => {
    render(meMock({ superuser: false, permissions: ['orders.view'] }))
    expect(await screen.findByText('No orders yet.')).toBeDefined()
    expect(screen.queryByRole('button', { name: /Stage/ })).toBeNull()
  })

  it('offers a plugin filter in the toolbar and hands the pick to the server', async () => {
    const picked = {
      request: {
        query: OrdersPagedDocument,
        variables: (v: Record<string, unknown>) =>
          JSON.stringify(v.pluginFilters) === JSON.stringify([{ key: 'acme:sync', values: ['failed'] }]),
      },
      result: { data: { ordersPaged: { __typename: 'OrderPage', totalCount: 1, items: [{
        __typename: 'Order', id: 'o9', number: '1009', status: 'PROCESSING', paymentStatus: 'PAID', total: 1000, currencyCode: 'EUR',
        createdAt: '2026-01-01T00:00:00Z', stage: null, shippingAddress: { __typename: 'OrderAddress', firstName: 'Ada', lastName: 'Buyer' },
      }] } } },
      maxUsageCount: 5,
    }
    const filters = pluginFiltersMock([{ key: 'acme:sync', label: 'ERP sync', options: [{ value: 'failed', label: 'Failed' }] }])
    render(meMock({ superuser: false, permissions: ['orders.view'] }), [picked, empty, stagesMock([]), filters])
    expect(await screen.findByText('No orders yet.')).toBeDefined()
    fireEvent.click(await screen.findByRole('button', { name: /ERP sync/ }))
    fireEvent.click(await screen.findByRole('option', { name: /Failed/ }))
    expect(await screen.findByText('1009')).toBeDefined()
  })

  // A link saved while a plugin was on, opened after it was switched off - the server refuses a filter it doesn't
  // offer, so the pick is left out and the list still loads
  it('leaves out a plugin filter the list no longer offers', async () => {
    // Answers only a request without the stale pick, which is what the server itself would do
    const withoutIt = {
      request: { query: OrdersPagedDocument, variables: (v: Record<string, unknown>) => v.pluginFilters == null },
      result: { data: { ordersPaged: { __typename: 'OrderPage', totalCount: 1, items: [{
        __typename: 'Order', id: 'o10', number: '1010', status: 'PROCESSING', paymentStatus: 'PAID', total: 1000, currencyCode: 'EUR',
        createdAt: '2026-01-01T00:00:00Z', stage: null, shippingAddress: { __typename: 'OrderAddress', firstName: 'Ada', lastName: 'Buyer' },
      }] } } },
      maxUsageCount: 5,
    }
    const stale = encodeURIComponent(JSON.stringify([{ id: 'plugin:acme:sync', value: ['failed'] }]))
    renderPage({
      path: `/orders?f=${stale}`,
      routes: { '/orders': <OrdersList /> },
      mocks: [meMock({ superuser: false, permissions: ['orders.view'] }), withoutIt, stagesMock([]), pluginFiltersMock([])],
      adminId: 'admin-1',
    })
    expect(await screen.findByText('1010')).toBeDefined()
  })
})
