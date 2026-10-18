import { describe, expect, it } from 'vitest'
import { toAddressValues } from '@/shared/schemas/address'
import { cartOfferSchema, offerDiff, toOfferInput, type CartOfferValues } from './cartOffer'
import { customLineSchema } from './customLine'
import { newCartSchema } from './newCart'

const address = { ...toAddressValues(), firstName: 'Ana', lastName: 'Ilić', street1: 'Main 1', city: 'Beograd', postalCode: '11000', countryCode: 'RS' }

const base: CartOfferValues = {
  expiresAt: '2030-01-01T12:00',
  message: '',
  showInAccount: false,
  locks: [],
  hasShipping: false,
  shipping: toAddressValues(),
  billingSameAsShipping: false,
  hasBilling: false,
  billing: toAddressValues(),
  paymentMethods: [],
  deliveries: [],
}

describe('cart offer schema', () => {
  it('needs what a lock fixes to be filled in', () => {
    const paths = (v: CartOfferValues) => cartOfferSchema.safeParse(v).error?.issues.map((i) => i.path.join('.'))
    expect(paths(base)).toBeUndefined()
    expect(paths({ ...base, locks: ['SHIPPING'] })).toEqual(['locks'])
    expect(paths({ ...base, locks: ['BILLING'], hasShipping: true, shipping: address, billingSameAsShipping: true })).toBeUndefined()
    expect(paths({ ...base, hasShipping: true })).toContain('shipping.street1')
    expect(paths({ ...base, expiresAt: '' })).toEqual(['expiresAt'])
  })
})

describe('offer input', () => {
  it('drops the shipping parts of a cart that doesn’t ship', () => {
    const v = { ...base, hasShipping: true, shipping: address, billingSameAsShipping: true, locks: ['SHIPPING', 'LINES'] as CartOfferValues['locks'] }
    const input = toOfferInput(v, false)
    expect(input.shippingAddress).toBeNull()
    expect(input.billingSameAsShipping).toBe(false)
    expect(input.locks).toEqual(['LINES'])
    expect(input.deliveries).toEqual([])
  })

  it('sends billing as the shipping address when it mirrors it', () => {
    const input = toOfferInput({ ...base, hasShipping: true, shipping: address, billingSameAsShipping: true, hasBilling: true, billing: address }, true)
    expect(input.shippingAddress?.street1).toBe('Main 1')
    expect(input.billingSameAsShipping).toBe(true)
    expect(input.billingAddress).toBeNull()
  })
})

describe('offer diff', () => {
  it('lists one readable change per edit', () => {
    const diff = offerDiff(base, { ...base, locks: ['LINES', 'COUPONS'], paymentMethods: ['cod', 'bank'], hasShipping: true, shipping: address })
    expect(diff.map((d) => d.label)).toEqual(['Locked', 'Shipping address', 'Payment methods'])
    expect(diff[0].after).toBe('Items and quantities, Coupons, gift cards and store credit')
    expect(diff[2]).toMatchObject({ before: 'Any the store offers', after: 'cod, bank' })
    expect(diff[1].after).toBe('Ana Ilić, Main 1, 11000 Beograd, RS')
  })

  it('is empty when nothing changed', () => {
    expect(offerDiff(base, { ...base, locks: [] })).toEqual([])
  })
})

describe('custom line schema', () => {
  it('needs a name, a price and a positive quantity', () => {
    const paths = (v: Parameters<typeof customLineSchema.safeParse>[0]) => customLineSchema.safeParse(v).error?.issues.map((i) => i.path.join('.'))
    expect(paths({ name: 'Fee', price: '12.50', quantity: '1', taxClass: '' })).toBeUndefined()
    expect(paths({ name: ' ', price: 'x', quantity: '0', taxClass: '' })).toEqual(['name', 'price', 'quantity'])
  })
})

describe('new cart schema - offers', () => {
  it('needs the recipient’s email for a guest offer', () => {
    const v = { mode: 'guest' as const, customer: null, email: '', firstName: '', lastName: '', offer: true, guestEmail: 'nope' }
    expect(newCartSchema.safeParse(v).error?.issues.map((i) => i.path.join('.'))).toEqual(['guestEmail'])
    expect(newCartSchema.safeParse({ ...v, guestEmail: 'a@b.co' }).success).toBe(true)
    expect(newCartSchema.safeParse({ ...v, offer: false }).success).toBe(true)
  })
})
