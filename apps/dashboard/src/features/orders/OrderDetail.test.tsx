import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import type { MockLink } from '@apollo/client/testing'
import { describe, expect, it } from 'vitest'
import { SLOW, meMock, renderPage } from '@/test/page-harness'
import { CancelOrderDocument, OrderDocument } from './Orders.gql'
import { OrderDetail } from './OrderDetail'

// What this pins: cancelling an order goes through the shared ConfirmDialog - a refused cancel keeps
// it open with the server's message, a successful one closes it and the page shows the order cancelled

const address = {
  __typename: 'Address' as const,
  firstName: 'Ada',
  lastName: 'Buyer',
  company: null,
  street1: '1 Main St',
  street2: null,
  city: 'Berlin',
  state: null,
  postalCode: '10115',
  countryCode: 'DE',
  phone: null,
}

// The smallest order the page renders: pending, unpaid, no lines, shipments or payments
const order = {
  __typename: 'Order' as const,
  id: 'o1',
  channelId: 'c1',
  number: '1001',
  status: 'PENDING' as const,
  paymentStatus: 'UNPAID' as const,
  paymentProvider: 'manual',
  paymentReference: '',
  email: 'ada@example.com',
  customerId: null,
  subtotal: 1000,
  shippingAmount: 0,
  discountAmount: 0,
  taxAmount: 0,
  taxIncluded: false,
  shippingRefundable: 0,
  total: 1000,
  giftCardAmount: 0,
  storeCreditAmount: 0,
  currencyCode: 'EUR',
  customerNote: null,
  poNumber: null,
  businessIds: [],
  checkoutFields: [] as { __typename: 'OrderCheckoutField'; key: string; label: string; value: string }[],
  carrier: '',
  trackingNumber: '',
  trackingUrl: '',
  createdAt: '2026-01-01T00:00:00Z',
  billingAddress: address,
  shippingAddress: address,
  lines: [],
  events: [],
  fulfillments: [],
  paymentTransactions: [],
  taxLines: [],
  appliedPromotions: [],
  returns: [],
}

describe('OrderDetail cancel', () => {
  async function confirmCancel(mock: MockLink.MockedResponse) {
    renderPage({
      path: '/orders/o1',
      routes: { '/orders/:id': <OrderDetail /> },
      mocks: [meMock(), { request: { query: OrderDocument, variables: { id: 'o1' } }, result: { data: { order } }, maxUsageCount: 3 }, mock],
      adminId: 'admin-1',
    })
    fireEvent.click(await screen.findByRole('button', { name: 'Cancel' }, SLOW))
    const dialog = await screen.findByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: 'Cancel order' }))
    return dialog
  }

  it('keeps the dialog open and shows a refused cancel', async () => {
    const dialog = await confirmCancel({ request: { query: CancelOrderDocument, variables: { id: 'o1' } }, result: { errors: [{ message: 'order already shipped' }] } })
    expect(await within(dialog).findByText('order already shipped', {}, SLOW)).toBeDefined()
    expect(screen.getByRole('alertdialog')).toBe(dialog)
  })

  it('closes the dialog and shows the order cancelled', async () => {
    await confirmCancel({
      request: { query: CancelOrderDocument, variables: { id: 'o1' } },
      result: { data: { cancelOrder: { __typename: 'Order', id: 'o1', status: 'CANCELLED' } } },
    })
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull(), SLOW)
    expect(await screen.findByText('The order was cancelled, so nothing more will ship.', {}, SLOW)).toBeDefined()
  })
})

describe('OrderDetail checkout answers', () => {
  it("shows what the store's plugins asked at checkout, a ticked box as Yes", async () => {
    const answered = {
      ...order,
      checkoutFields: [
        { __typename: 'OrderCheckoutField' as const, key: 'gift_message', label: 'Gift message', value: 'Happy birthday' },
        { __typename: 'OrderCheckoutField' as const, key: 'wrap', label: 'Gift wrap', value: 'true' },
      ],
    }
    renderPage({
      path: '/orders/o1',
      routes: { '/orders/:id': <OrderDetail /> },
      mocks: [meMock(), { request: { query: OrderDocument, variables: { id: 'o1' } }, result: { data: { order: answered } }, maxUsageCount: 3 }],
      adminId: 'admin-1',
    })
    expect(await screen.findByText('At checkout', {}, SLOW)).toBeDefined()
    expect(screen.getByText('Happy birthday')).toBeDefined()
    expect(screen.getByText('Gift wrap').nextElementSibling?.textContent).toBe('Yes')
  })
})
