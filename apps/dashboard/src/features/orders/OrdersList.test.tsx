import { screen, waitFor } from '@testing-library/react'
import type { MockLink } from '@apollo/client/testing'
import { describe, expect, it } from 'vitest'
import { meMock, renderPage } from '@/test/page-harness'
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

function render(me: ReturnType<typeof meMock>, mocks: MockLink.MockedResponse[] = [empty, stagesMock([])]) {
  // The table keeps its column and density choices as the admin's preferences
  renderPage({ path: '/orders', routes: { '/orders': <OrdersList /> }, mocks: [me, ...mocks], adminId: 'admin-1' })
}

describe('OrdersList', () => {
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
    render(meMock({ superuser: false, permissions: ['orders.view'] }), [staged, stagesMock([{ key: 'awaiting_stock', label: 'Awaiting stock', within: ['PROCESSING'] }])])
    expect(await screen.findByText('Awaiting stock')).toBeDefined()
    expect(screen.getByRole('button', { name: /Stage/ })).toBeDefined()
  })

  it('leaves the stage column and filter out when no plugin offers one', async () => {
    render(meMock({ superuser: false, permissions: ['orders.view'] }))
    expect(await screen.findByText('No orders yet.')).toBeDefined()
    expect(screen.queryByRole('button', { name: /Stage/ })).toBeNull()
  })
})
