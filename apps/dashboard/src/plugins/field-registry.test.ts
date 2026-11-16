import { afterEach, describe, expect, it } from 'vitest'
import { __resetDetailFieldsForTests, detailFieldFor, detailFieldText, getDetailFields, registerDetailField } from './field-registry'

afterEach(() => __resetDetailFieldsForTests())

describe('detail field registry', () => {
  it('lists a target’s fields in order and refuses clashes', () => {
    registerDetailField({ id: 'acme:tier', target: 'customer', key: 'tier', label: 'Tier', order: 20 })
    registerDetailField({ id: 'acme:vip', target: 'customer', key: 'vip', label: 'VIP', type: 'checkbox', order: 10 })
    registerDetailField({ id: 'acme:tier-product', target: 'product', key: 'tier', label: 'Tier' })
    expect(getDetailFields('customer').map((f) => f.id)).toEqual(['acme:vip', 'acme:tier'])
    expect(getDetailFields(undefined)).toEqual([])
    expect(detailFieldFor('product', 'tier')?.id).toBe('acme:tier-product')

    expect(() => registerDetailField({ id: 'acme:tier', target: 'country', key: 'x', label: 'X' })).toThrow(/duplicate id/)
    expect(() => registerDetailField({ id: 'other:tier', target: 'customer', key: 'tier', label: 'T' })).toThrow(/already has a field/)
    expect(() => registerDetailField({ id: 'other:size', target: 'customer', key: 'size', label: 'S', type: 'select' })).toThrow(/needs options/)
    expect(() => registerDetailField({ id: 'other:blank', target: 'customer', key: ' ', label: 'B' })).toThrow(/metadata key/)
    expect(() => registerDetailField({ id: 'other:long', target: 'customer', key: 'k'.repeat(101), label: 'L' })).toThrow(/metadata key/)
    expect(() => registerDetailField({ id: 'other:spaced', target: 'customer', key: 'gift note', label: 'G' })).toThrow(/metadata key/)
  })

  it('reads a value the way the field shows it', () => {
    expect(detailFieldText({ id: 'a', target: 't', key: 'k', label: 'L', type: 'checkbox' }, 'true')).toBe('Yes')
    expect(detailFieldText({ id: 'a', target: 't', key: 'k', label: 'L', type: 'select', options: [{ value: 'gold', label: 'Gold' }] }, 'gold')).toBe('Gold')
    expect(detailFieldText({ id: 'a', target: 't', key: 'k', label: 'L' }, 'plain')).toBe('plain')
  })
})
