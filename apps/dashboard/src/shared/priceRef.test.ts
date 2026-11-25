import { describe, expect, it } from 'vitest'
import { priceRefLabel } from './priceRef'

describe('priceRefLabel', () => {
  it('names the plugin or webhook that set a price', () => {
    expect(priceRefLabel('plugin:member-prices')).toBe('member-prices plugin')
    expect(priceRefLabel('webhook:quotes')).toBe('quotes webhook')
    expect(priceRefLabel('webhook:erp:eu')).toBe('erp:eu webhook')
  })
  it('says nothing when nobody is named', () => {
    expect(priceRefLabel('')).toBe('')
    expect(priceRefLabel('plugin:')).toBe('')
  })
})
