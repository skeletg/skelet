import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import type { MockLink } from '@apollo/client/testing'
import type { OrderCheckoutFieldType } from '@/graphql/schema.types'
import { describe, expect, it } from 'vitest'
import { SLOW, meMock, renderPage } from '@/test/page-harness'
import { CancelOrderDocument, OrderDocument, OrderStagesDocument, SetOrderStageDocument } from './Orders.gql'
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
  stage: null,
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
  feesRefundable: 0,
  fees: [],
  total: 1000,
  giftCardAmount: 0,
  storeCreditAmount: 0,
  currencyCode: 'EUR',
  customerNote: null,
  poNumber: null,
  businessIds: [],
  checkoutFields: [] as { __typename: 'OrderCheckoutField'; key: string; label: string; value: string; type: OrderCheckoutFieldType }[],
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

const stagesMock = { request: { query: OrderStagesDocument, variables: { channelId: 'c1' } }, result: { data: { orderStages: [] } }, maxUsageCount: 3 }

describe('OrderDetail cancel', () => {
  async function confirmCancel(mock: MockLink.MockedResponse) {
    renderPage({
      path: '/orders/o1',
      routes: { '/orders/:id': <OrderDetail /> },
      mocks: [meMock(), { request: { query: OrderDocument, variables: { id: 'o1' } }, result: { data: { order } }, maxUsageCount: 3 }, stagesMock, mock],
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
  it("shows what the store's plugins asked at checkout, each the way its field reads", async () => {
    const answered = {
      ...order,
      checkoutFields: [
        { __typename: 'OrderCheckoutField' as const, key: 'acme:gift_message', label: 'Gift message', value: 'Happy birthday', type: 'TEXTAREA' as const },
        { __typename: 'OrderCheckoutField' as const, key: 'acme:wrap', label: 'Gift wrap', value: 'true', type: 'CHECKBOX' as const },
        { __typename: 'OrderCheckoutField' as const, key: 'acme:motto', label: 'Motto', value: 'true', type: 'TEXT' as const },
      ],
    }
    renderPage({
      path: '/orders/o1',
      routes: { '/orders/:id': <OrderDetail /> },
      mocks: [meMock(), { request: { query: OrderDocument, variables: { id: 'o1' } }, result: { data: { order: answered } }, maxUsageCount: 3 }, stagesMock],
      adminId: 'admin-1',
    })
    expect(await screen.findByText('At checkout', {}, SLOW)).toBeDefined()
    expect(screen.getByText('Happy birthday')).toBeDefined()
    expect(screen.getByText('Gift wrap').nextElementSibling?.textContent).toBe('Yes')
    expect(screen.getByText('Motto').nextElementSibling?.textContent).toBe('true')
  })
})

describe('OrderDetail stage', () => {
  it('moves the order into a stage its status holds, and offers none that it does not', async () => {
    const processing = { ...order, status: 'PROCESSING' as const }
    const moved = { request: { query: SetOrderStageDocument, variables: { id: 'o1', stage: 'awaiting_stock' } }, result: { data: { setOrderStage: {
      __typename: 'Order', id: 'o1', status: 'PROCESSING', stage: { __typename: 'OrderStage', key: 'awaiting_stock', label: 'Awaiting stock' },
    } } } }
    const stages = { ...stagesMock, result: { data: { orderStages: [
      { __typename: 'OrderStage', key: 'awaiting_stock', label: 'Awaiting stock', within: ['CONFIRMED', 'PROCESSING'] },
      { __typename: 'OrderStage', key: 'delivered', label: 'Delivered', within: ['FULFILLED'] },
    ] } } }
    renderPage({
      path: '/orders/o1',
      routes: { '/orders/:id': <OrderDetail /> },
      mocks: [meMock(), { request: { query: OrderDocument, variables: { id: 'o1' } }, result: { data: { order: processing } }, maxUsageCount: 3 }, stages, moved],
      adminId: 'admin-1',
    })
    fireEvent.pointerDown(await screen.findByRole('button', { name: 'Stage' }, SLOW), { button: 0, ctrlKey: false, pointerType: 'mouse' })
    const awaiting = await screen.findByRole('menuitem', { name: /Awaiting stock/ })
    expect(screen.queryByRole('menuitem', { name: /Delivered/ })).toBeNull()
    fireEvent.click(awaiting)
    expect(await screen.findByText('Awaiting stock', { selector: '[data-slot="badge"]' }, SLOW)).toBeDefined()
  })
})

describe('OrderDetail lines', () => {
  const line = (id: string, name: string, more: Record<string, unknown>) => ({
    __typename: 'OrderLine' as const, id, name, sku: id.toUpperCase(), unitPrice: 5000, quantity: 2, totalPrice: 10000,
    discountAmount: 0, taxAmount: 0, priceSource: 'CATALOG' as const, listUnitPrice: null, priceReason: '',
    isGift: false, components: [], options: [], ...more,
  })
  const show = (taxIncluded: boolean, lines = [line('shirt', 'Shirt', { discountAmount: 2000 }), line('book', 'Book', { taxAmount: 500 })]) =>
    renderPage({
      path: '/orders/o1',
      routes: { '/orders/:id': <OrderDetail /> },
      mocks: [meMock(), { request: { query: OrderDocument, variables: { id: 'o1' } }, result: { data: { order: { ...order, taxIncluded, lines } } }, maxUsageCount: 3 }, stagesMock],
      adminId: 'admin-1',
    })

  it('shows each line its own discount and tax, and neither where it has none', async () => {
    show(false)
    const shirt = (await screen.findByText('Shirt', {}, SLOW)).closest('tr')
    const book = screen.getByText('Book').closest('tr')
    expect(shirt?.textContent).toMatch(/-.*20\.00 discount/)
    expect(shirt?.textContent).not.toMatch(/tax/)
    expect(book?.textContent).toMatch(/\+.*5\.00 tax/)
    expect(book?.textContent).not.toMatch(/discount/)
  })

  it('shows a price an admin set beside the catalog price it replaced, with the note left for it', async () => {
    show(false, [
      line('shirt', 'Shirt', { priceSource: 'ADMIN', listUnitPrice: 6500, priceReason: 'Trade price' }),
      line('fee', 'Setup fee', { priceSource: 'ADMIN' }),
      line('book', 'Book', {}),
    ])
    const shirt = (await screen.findByText('Shirt', {}, SLOW)).closest('tr')
    expect(shirt?.querySelector('s')?.textContent).toMatch(/65\.00/)
    expect(shirt?.textContent).toContain('Trade price')
    expect(screen.getByText('Setup fee').closest('tr')?.querySelector('s')).toBeNull()
    expect(screen.getByText('Book').closest('tr')?.querySelector('s')).toBeNull()
  })

  it('says the tax is in the price where the order was priced that way', async () => {
    show(true)
    const book = (await screen.findByText('Book', {}, SLOW)).closest('tr')
    expect(book?.textContent).toMatch(/incl\..*5\.00 tax/)
  })
})
