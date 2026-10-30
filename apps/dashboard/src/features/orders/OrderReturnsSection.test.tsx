import { fireEvent, screen } from '@testing-library/react'
import { useQuery } from '@apollo/client/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, meMock, renderDialog } from '@/test/page-harness'
import { OrderReturnsSection } from './OrderReturnsSection'
import { ApproveReturnDocument, OrderDocument, OrdersPagedDocument, RequestReturnDocument, type OrdersPagedQueryVariables } from './Orders.gql'

// What these pin: a return action refetches the orders list by its generated document - whichever
// copy is mounted, with that copy's own filter and page - and fires no list request when none is

const order = {
  id: 'o1',
  channelId: 'c1',
  status: 'FULFILLED',
  currencyCode: 'EUR',
  lines: [{ id: 'l1', name: 'Tee', sku: 'TEE-1', quantity: 1, components: [] }],
  returns: [
    {
      id: 'r1',
      rmaNumber: 'RMA-1',
      status: 'requested',
      refundAmount: 0,
      refundTarget: 'original',
      restock: true,
      reason: null,
      createdAt: '2026-01-01T00:00:00Z',
      lines: [{ id: 'rl1', orderLineId: 'l1', quantity: 1, refundUnitCents: 1000 }],
    },
  ],
} as never

// Page two of the paid orders - variables no mutation site could know
const listVariables: OrdersPagedQueryVariables = { statuses: ['PAID'], limit: 25, offset: 25 }

// Stands in for OrdersList: watches the list under its own variables
function OrdersListProbe() {
  useQuery(OrdersPagedDocument, { variables: listVariables })
  return null
}

const recorder = (calls: Record<string, unknown>[]) => (vars: Record<string, unknown>) => {
  calls.push(vars)
  return true
}

function renderReturns({ withList }: { withList: boolean }) {
  const listRequests: Record<string, unknown>[] = []
  const detailRequests: Record<string, unknown>[] = []
  renderDialog(
    <>
      {withList && <OrdersListProbe />}
      <OrderReturnsSection order={order} />
    </>,
    [
      meMock(),
      {
        request: { query: OrdersPagedDocument, variables: recorder(listRequests) },
        result: { data: { ordersPaged: { __typename: 'OrderPage', totalCount: 0, items: [] } } },
        maxUsageCount: 5,
      },
      {
        request: { query: ApproveReturnDocument, variables: { id: 'r1' } },
        result: { data: { approveReturn: { __typename: 'OrderReturn', id: 'r1', status: 'approved' } } },
      },
      // Fired by the same refetch as the list, so once it lands the list request would have too
      {
        request: { query: OrderDocument, variables: recorder(detailRequests) },
        result: { data: { order: null } },
        maxUsageCount: 3,
      },
    ],
  )
  return { listRequests, detailRequests }
}

describe('OrderReturnsSection', () => {
  it('refetches the mounted orders list with its own variables', async () => {
    const { listRequests } = renderReturns({ withList: true })
    await vi.waitFor(() => expect(listRequests).toHaveLength(1), SLOW)

    fireEvent.click(await screen.findByRole('button', { name: 'Approve' }, SLOW))
    await vi.waitFor(() => expect(listRequests).toHaveLength(2), SLOW)
    expect(listRequests[1]).toEqual(listVariables)
  })

  it('fires no list request when no list is mounted', async () => {
    const { listRequests, detailRequests } = renderReturns({ withList: false })
    fireEvent.click(await screen.findByRole('button', { name: 'Approve' }, SLOW))
    await vi.waitFor(() => expect(detailRequests).toEqual([{ id: 'o1' }]), SLOW)
    expect(listRequests).toHaveLength(0)
    expect(screen.queryByText('Return action failed')).toBeNull()
  })
})

// What this pins: one item can come back out of a bundle - the request names the component and counts its units
describe('returning one component of a bundle', () => {
  it('sends the component with its quantity', async () => {
    const sent: Record<string, unknown>[] = []
    const bundleOrder = {
      id: 'o2', channelId: 'c1', status: 'FULFILLED', currencyCode: 'EUR', returns: [],
      lines: [{ id: 'l1', name: 'Gift box', sku: 'BOX', quantity: 2, components: [{ id: 'k1', name: 'Mug', sku: 'MUG', unitPrice: 600, quantity: 2 }] }],
    } as never
    renderDialog(<OrderReturnsSection order={bundleOrder} />, [
      meMock(),
      { request: { query: RequestReturnDocument, variables: recorder(sent) }, result: { data: { requestReturn: { __typename: 'OrderReturn', id: 'r9' } } } },
      { request: { query: OrderDocument, variables: () => true }, result: { data: { order: null } }, maxUsageCount: 3 },
      { request: { query: OrdersPagedDocument, variables: () => true }, result: { data: { ordersPaged: { __typename: 'OrderPage', totalCount: 0, items: [] } } }, maxUsageCount: 3 },
    ])
    fireEvent.click(await screen.findByRole('button', { name: 'New return' }, SLOW))
    fireEvent.click(await screen.findByRole('checkbox', { name: 'Return Mug on its own' }))
    fireEvent.click(screen.getByRole('button', { name: 'Open return' }))
    await vi.waitFor(() => expect(sent).toHaveLength(1), SLOW)
    expect((sent[0].input as { lines: unknown[] }).lines).toEqual([{ orderLineId: 'l1', componentId: 'k1', quantity: 1 }])
  })
})
