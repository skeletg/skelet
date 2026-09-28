import { describe, expect, it } from 'vitest'
import { paramsSummary, taxRuleDiff, taxRuleSchema, toTaxRuleInput, toTaxRuleValues } from './taxRule'

const stored = {
  id: 'r1',
  channelId: 'c1',
  name: 'CA sales tax',
  priority: 0,
  exclusivityGroup: 'jurisdiction',
  sortOrder: 0,
  isActive: true,
  conditions: [{ type: 'shipping_country', params: { countries: ['US'] } }],
  effects: [{ type: 'rate', params: { rate: 7 } }],
}

describe('tax rule schema', () => {
  it('accepts a stored rule unchanged', () => {
    expect(taxRuleSchema.safeParse(toTaxRuleValues(stored)).success).toBe(true)
  })

  it('notices a params-only edit, which the old type-list diff missed', () => {
    const before = toTaxRuleValues(stored)
    const after = { ...before, effects: [{ type: 'rate', params: { rate: 7.25 } }] }
    expect(taxRuleDiff(before, after)).toEqual([
      { kind: 'change', label: 'Effects · 1', before: 'rate (rate: 7)', after: 'rate (rate: 7.25)' },
    ])
  })

  it('lists added and removed rows', () => {
    const before = toTaxRuleValues(stored)
    const after = { ...before, conditions: [], effects: [...before.effects, { type: 'flat', params: {} }] }
    expect(taxRuleDiff(before, after)).toEqual([
      { kind: 'remove', label: 'Conditions · 1', before: 'shipping_country (countries: US)', after: '' },
      { kind: 'add', label: 'Effects · 2', before: '', after: 'flat' },
    ])
  })

  it('summarises params without dumping JSON', () => {
    expect(paramsSummary({ countries: ['US', 'DE'], inclusive: true })).toBe('countries: US, DE; inclusive: yes')
  })

  it('sends row order as sortOrder', () => {
    expect(toTaxRuleInput(toTaxRuleValues(stored)).effects).toEqual([{ type: 'rate', params: { rate: 7 }, sortOrder: 0 }])
  })
})
