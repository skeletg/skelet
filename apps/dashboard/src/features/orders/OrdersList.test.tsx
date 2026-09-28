import { screen, waitFor } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { meMock, renderPage } from '@/test/page-harness'
import { OrdersList } from './OrdersList'
import { OrdersPagedDocument } from './Orders.gql'

// What these pin: the header only offers what the admin can open - New order needs orders.update and
// Export CSV the data hub's data.view, the same permissions their routes check. A viewer used to get
// both buttons, each leading to an access-restricted page

const empty = {
  request: { query: OrdersPagedDocument, variables: () => true },
  result: { data: { ordersPaged: { __typename: 'OrderPage', totalCount: 0, items: [] } } },
  maxUsageCount: 5,
}

function render(me: ReturnType<typeof meMock>) {
  // The table keeps its column and density choices as the admin's preferences
  renderPage({ path: '/orders', routes: { '/orders': <OrdersList /> }, mocks: [me, empty], adminId: 'admin-1' })
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
})
