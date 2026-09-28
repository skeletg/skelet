import { describe, expect, it } from 'vitest'
import { currencyDiff, currencySchema, exchangeRateOverride, toCurrencyValues } from './currency'

const stored = {
  code: 'EUR',
  name: 'Euro',
  symbol: '€',
  decimals: 2,
  symbolPosition: 'suffix',
  isActive: true,
  sortOrder: 1,
  exchangeRate: 0.92,
  metadata: [],
}

describe('currency schema', () => {
  it('accepts a stored record unchanged', () => {
    expect(currencySchema.safeParse(toCurrencyValues(stored)).success).toBe(true)
  })

  it('bounds decimals and wants a positive rate', () => {
    const issues = currencySchema
      .safeParse({ ...toCurrencyValues(stored), decimals: 7, exchangeRate: '-1' })
      .error?.issues.map((i) => i.path.join('.'))
    expect(issues).toEqual(['decimals', 'exchangeRate'])
  })

  it('sends a rate only when the operator changed it', () => {
    const values = toCurrencyValues(stored)
    expect(exchangeRateOverride(values, stored.exchangeRate)).toEqual({})
    expect(exchangeRateOverride({ ...values, exchangeRate: '' }, stored.exchangeRate)).toEqual({})
    expect(exchangeRateOverride({ ...values, exchangeRate: '0.95' }, stored.exchangeRate)).toEqual({ exchangeRate: 0.95 })
  })

  it('diffs the fields and metadata', () => {
    const before = toCurrencyValues(stored)
    const after = { ...before, decimals: 0, metadata: [{ key: 'iso', value: '978' }] }
    expect(currencyDiff(before, after)).toEqual([
      { kind: 'change', label: 'Decimals', before: '2', after: '0' },
      { kind: 'add', label: 'Metadata · iso', before: '', after: '978' },
    ])
  })
})
