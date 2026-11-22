import { fireEvent, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, meMock, renderPage } from '@/test/page-harness'
import { CartDetail } from './CartDetail'
import { AdminSetCartLinePriceDocument, CartDocument } from './Carts.gql'
import { priceFloor } from './schemas/setPrice'

// What these pin: a price on a shopper's own cart is set through its own permission, always with a reason, and the
// dialog says what the store allows before the admin types a price it would refuse

const line = {
  __typename: 'AdminCartLine' as const, id: 'l1', name: 'Rain jacket', sku: 'JACKET-M', unitPrice: 10000, quantity: 2, totalPrice: 20000,
  isSimple: true, priceOverride: false, priceSource: 'CATALOG' as const, listUnitPrice: null, priceReason: '', priceMaxQuantity: null, priceExpiresAt: null, promotionsApply: true,
  isCustom: false, taxClass: '', components: [], options: [],
}
const cart = {
  __typename: 'AdminCart' as const, id: 'cart-1', customerId: null, customerEmail: null, customerName: null, channelId: 'c1', currencyCode: 'EUR',
  status: 'active', source: 'web', kind: 'SHOPPER' as const, email: 'guest@example.com', subtotal: 20000, itemCount: 1,
  createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-01T00:00:00Z', offer: null,
  priceLimits: { __typename: 'CartPriceLimits' as const, maxCutPercent: 20, holdsUntil: '2030-03-05T12:00:00Z' },
  lines: [line],
}

function renderCart(opts: { me?: Parameters<typeof meMock>[0]; cart?: Record<string, unknown> } = {}) {
  const set = vi.fn()
  renderPage({
    path: '/carts/cart-1',
    routes: { '/carts/:id': <CartDetail /> },
    mocks: [
      meMock(opts.me),
      { request: { query: CartDocument, variables: { id: 'cart-1' } }, result: { data: { cart: { ...cart, ...opts.cart } } }, maxUsageCount: 5 },
      {
        request: { query: AdminSetCartLinePriceDocument, variables: (v: Record<string, unknown>) => { set(v); return true } },
        result: { data: { adminSetCartLinePrice: { ...cart, lines: [{ ...line, unitPrice: 8500, priceOverride: true, priceSource: 'ADMIN', listUnitPrice: 10000, priceReason: 'Price match', priceMaxQuantity: 2 }] } } },
      },
    ],
    adminId: 'admin-1',
  })
  return set
}

describe('priceFloor', () => {
  const limits = (maxCutPercent: number) => ({ maxCutPercent })
  it('is the catalog price less the cut, rounded up to a cent the backend takes', () => {
    expect(priceFloor(10000, limits(20))).toBe(8000)
    expect(priceFloor(999, limits(15))).toBe(850)
    expect(priceFloor(10000, limits(0))).toBe(10000)
  })
  it('is none when any price goes, or the line has no catalog price', () => {
    expect(priceFloor(10000, limits(100))).toBeNull()
    expect(priceFloor(null, limits(20))).toBeNull()
  })
})

describe('setting a price on a shopper cart', () => {
  it('asks for a reason, says what the store allows, and sends the price with it', async () => {
    const set = renderCart()
    fireEvent.click(await screen.findByRole('button', { name: 'Set price' }, SLOW))
    const dialog = await screen.findByRole('dialog')
    expect(dialog.textContent).toContain('for up to 2 of the item')
    expect(dialog.textContent).toMatch(/The catalog price is .*100\.00.*The lowest you can set is .*80\.00/)

    fireEvent.change(screen.getByLabelText('Unit price'), { target: { value: '85' } })
    fireEvent.click(screen.getByRole('button', { name: 'Set price' }))
    expect(await screen.findByText('Say why the price is being set')).toBeDefined()
    expect(set).not.toHaveBeenCalled()

    fireEvent.change(screen.getByLabelText('Reason'), { target: { value: '  Price match ' } })
    fireEvent.click(screen.getByRole('button', { name: 'Set price' }))
    await waitFor(() => expect(set).toHaveBeenCalledWith({ cartId: 'cart-1', lineId: 'l1', unitPrice: 8500, reason: 'Price match', promotionsApply: false }))
  })

  it('lets coupons and promotions apply only when the box is ticked', async () => {
    const set = renderCart()
    fireEvent.click(await screen.findByRole('button', { name: 'Set price' }, SLOW))
    await screen.findByRole('dialog')
    fireEvent.change(screen.getByLabelText('Unit price'), { target: { value: '85' } })
    fireEvent.change(screen.getByLabelText('Reason'), { target: { value: 'Price match' } })
    fireEvent.click(screen.getByRole('checkbox', { name: 'Coupons and promotions can lower it further' }))
    fireEvent.click(screen.getByRole('button', { name: 'Set price' }))
    await waitFor(() => expect(set).toHaveBeenCalledWith(expect.objectContaining({ unitPrice: 8500, promotionsApply: true })))
  })

  it('shows a set price with what it replaced, how many it is for and why', async () => {
    renderCart({ cart: { lines: [{ ...line, unitPrice: 8500, priceOverride: true, priceSource: 'ADMIN', listUnitPrice: 10000, priceReason: 'Price match', priceMaxQuantity: 2, priceExpiresAt: '2030-03-05T12:00:00Z', promotionsApply: false }] } })
    const row = (await screen.findByText('Rain jacket', {}, SLOW)).closest('div.rounded')
    expect(row?.textContent).toMatch(/set price/)
    expect(row?.textContent).toMatch(/catalog .*100\.00.*for up to 2/)
    expect(row?.textContent).toContain('Price match')
    expect(row?.textContent).toContain('no promotions')
    expect(screen.getByRole('button', { name: 'Change price' })).toBeDefined()
    expect(screen.getByRole('button', { name: 'Reset' })).toBeDefined()
  })

  it('is not offered without the permission, whatever else the admin may do to the cart', async () => {
    renderCart({ me: { superuser: false, permissions: ['orders.view', 'orders.update'] } })
    await screen.findByText('Rain jacket', {}, SLOW)
    expect(screen.queryByRole('button', { name: 'Set price' })).toBeNull()
  })
})
