import { fireEvent, screen, within } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { SLOW, meMock, renderPage } from '@/test/page-harness'
import { CartDetail } from './CartDetail'
import { AdminCartOfferPaymentMethodsDocument, AdminSendCartOfferDocument, AdminUpdateCartOfferDocument, CartDocument } from './Carts.gql'

// What these pin: an offer's settings stage behind the one header Save and go out as one
// adminUpdateCartOffer, and sending waits until nothing is staged

const offer = {
  __typename: 'AdminCartOffer' as const,
  state: 'DRAFT' as const,
  expiresAt: '2030-01-01T12:00:00Z',
  message: '',
  showInAccount: false,
  locks: [],
  paymentMethods: [],
  link: 'https://shop.example.com/offer/tok',
  linkPath: '/offer/tok',
  needsShipping: false,
  sentAt: null,
  sendCount: 0,
  openedAt: null,
  revokedAt: null,
  orderId: null,
  checkout: {
    __typename: 'AdminCartOfferCheckout' as const,
    shippingAddress: null,
    billingSameAsShipping: false,
    billingAddress: null,
    deliveries: [],
  },
}

const cart = {
  __typename: 'AdminCart' as const,
  id: 'cart-1',
  customerId: null,
  customerEmail: null,
  customerName: null,
  channelId: 'c1',
  currencyCode: 'EUR',
  status: 'active',
  kind: 'OFFER' as const,
  email: 'guest@example.com',
  subtotal: 5000,
  itemCount: 1,
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
  offer,
  lines: [
    {
      __typename: 'AdminCartLine' as const,
      id: 'l1',
      name: 'Extra payment',
      sku: 'CUSTOM-ABC',
      unitPrice: 5000,
      quantity: 1,
      totalPrice: 5000,
      isSimple: false,
      priceOverride: true,
      isCustom: true,
      taxClass: 'standard',
      components: [],
      options: [],
    },
  ],
}

function renderCart() {
  const writes = { update: vi.fn(), send: vi.fn() }
  renderPage({
    path: '/carts/cart-1',
    routes: { '/carts/:id': <CartDetail /> },
    mocks: [
      meMock(),
      { request: { query: CartDocument, variables: { id: 'cart-1' } }, result: { data: { cart } }, maxUsageCount: 5 },
      { request: { query: AdminCartOfferPaymentMethodsDocument, variables: { cartId: 'cart-1' } }, result: { data: { adminCartOfferPaymentMethods: [] } }, maxUsageCount: 5 },
      {
        request: {
          query: AdminUpdateCartOfferDocument,
          variables: (v: Record<string, unknown>) => {
            writes.update(v)
            return true
          },
        },
        result: { data: { adminUpdateCartOffer: { ...cart, offer: { ...offer, locks: ['LINES'], message: 'Thanks!' } } } },
      },
      {
        request: {
          query: AdminSendCartOfferDocument,
          variables: (v: Record<string, unknown>) => {
            writes.send(v)
            return true
          },
        },
        result: { data: { adminSendCartOffer: { ...cart, offer: { ...offer, state: 'SENT', sendCount: 1 } } } },
      },
    ],
  })
  return writes
}

afterEach(() => localStorage.clear())

describe('CartDetail - offers', () => {
  it('reviews the staged offer settings, then writes them in one update', async () => {
    const writes = renderCart()
    fireEvent.change(await screen.findByLabelText('Message', {}, SLOW), { target: { value: 'Thanks!' } })
    fireEvent.click(screen.getByRole('checkbox', { name: /Items and quantities/ }))
    // Shipping has nothing to lock on a cart that doesn't ship
    expect((screen.getByRole('checkbox', { name: /Shipping address/ }) as HTMLButtonElement).disabled).toBe(true)
    // Sending would mail the old terms while changes are staged
    expect((screen.getByRole('button', { name: 'Send' }) as HTMLButtonElement).disabled).toBe(true)

    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    const dialog = await screen.findByRole('alertdialog')
    expect(within(dialog).getByText('Message')).toBeDefined()
    expect(within(dialog).getByText('Locked')).toBeDefined()
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply 2 changes/ }))

    await vi.waitFor(() => expect(writes.update).toHaveBeenCalled(), SLOW)
    const { cartId, input } = writes.update.mock.calls[0][0] as { cartId: string; input: Record<string, unknown> }
    expect(cartId).toBe('cart-1')
    expect(input).toMatchObject({
      message: 'Thanks!',
      locks: ['LINES'],
      showInAccount: false,
      shippingAddress: null,
      billingAddress: null,
      paymentMethods: [],
      deliveries: [],
    })
    expect(new Date(input.expiresAt as string).toISOString()).toBe(input.expiresAt)
  })

  it('sends after a confirm', async () => {
    const writes = renderCart()
    fireEvent.click(await screen.findByRole('button', { name: 'Send' }, SLOW))
    const dialog = await screen.findByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: 'Send' }))
    await vi.waitFor(() => expect(writes.send).toHaveBeenCalledWith({ cartId: 'cart-1' }), SLOW)
  })
})
