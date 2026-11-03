import { describe, expect, it } from 'vitest'
import type { BundleOption, Selection } from './bundleStaging'
import { applyPricing, storeHealth, structureIssues } from './bundleHealth'

// What this pins: the dashboard flags what the cart would refuse, and prices the bundle the way add-to-cart sums it

const store = { isPublished: true, channel: { id: 'c1', name: 'EU', currencyCode: 'EUR' } }
const listed = (effectivePrice: number, inStock = true) => [{ channelId: 'c1', isPublished: true, effectivePrice, inStock }]
const sel = (id: string, listings: Selection['product']['listings'], extra: Partial<Selection> = {}): Selection => ({
  id, product: { id, name: id, kind: 'SIMPLE', defaultPrice: 0, defaultCurrencyCode: 'EUR', listings }, quantity: 1, isDefault: false, sortOrder: 0, ...extra,
})
const opt = (selections: Selection[], extra: Partial<BundleOption> = {}): BundleOption => ({
  id: 'o', inputType: 'RADIO', isRequired: true, sortOrder: 0, minSelections: 0, maxSelections: null, title: 'Shirt', content: [], selections, ...extra,
})

describe('bundle health', () => {
  it('prices a pick-one range with quantities and the defaults', () => {
    const [h] = storeHealth([store], [opt([sel('a', listed(500), { isDefault: true }), sel('b', listed(300), { quantity: 2 })])])
    expect(h.issues).toEqual([])
    expect(h.price).toEqual({ min: 500, max: 600, default: 500 })
  })

  it('prices a pick-several option between its minimum and its maximum', () => {
    const o = opt([sel('a', listed(100)), sel('b', listed(200)), sel('c', listed(400))], { inputType: 'CHECKBOX', minSelections: 2, maxSelections: 2 })
    expect(storeHealth([store], [o])[0].price).toEqual({ min: 300, max: 600, default: 0 })
  })

  it('flags an unlisted, an out-of-stock and an unbuyable default selection, and leaves them out of the price', () => {
    const [h] = storeHealth([store], [opt([sel('a', []), sel('b', listed(300, false), { isDefault: true }), sel('c', listed(700))])])
    expect(h.issues.map((i) => i.text)).toEqual([
      'a in “Shirt” isn’t sold on this store.',
      'Default b in “Shirt” is out of stock - customers open the bundle without it.',
    ])
    expect(h.price).toEqual({ min: 700, max: 700, default: 0 })
  })

  it('says a required option with nothing buyable blocks the bundle', () => {
    const [h] = storeHealth([store], [opt([sel('a', listed(300, false))])])
    expect(h.issues.at(-1)?.level).toBe('destructive')
    expect(h.price).toBeNull()
  })

  it('skips unsaved picks instead of calling them unlisted', () => {
    expect(storeHealth([store], [opt([sel('a', undefined)])])[0].issues).toEqual([])
  })

  it('flags an empty required option and a minimum above the selection count once, for every store', () => {
    const issues = structureIssues([opt([]), opt([sel('a', listed(1))], { inputType: 'CHECKBOX', title: 'Socks', minSelections: 2 })])
    expect(issues.map((i) => i.level)).toEqual(['destructive', 'destructive'])
  })
})

// What this pins: the price line follows the store's pricing mode with the cart's rounding, and says what the picks add up to
describe('bundle pricing modes', () => {
  it('rounds a percent off the sum the way the cart does', () => {
    expect(applyPricing(1010, { mode: 'SUM_MINUS_PERCENT', percent: 15 }, null)).toBe(858)
    expect(applyPricing(3100, { mode: 'SUM', percent: 15 }, null)).toBe(3100)
    expect(applyPricing(3100, { mode: 'FIXED', percent: 0 }, 2500)).toBe(2500)
  })

  it('prices 10% off and keeps the sum beside it', () => {
    const o = opt([sel('a', listed(500), { isDefault: true }), sel('b', listed(1000))])
    const [h] = storeHealth([{ ...store, bundlePricing: 'SUM_MINUS_PERCENT', bundleDiscountPercent: 10 }], [o])
    expect(h.price).toEqual({ min: 450, max: 900, default: 450 })
    expect(h.sum).toEqual({ min: 500, max: 1000 })
  })

  it('flags a fixed price the store has no price for', () => {
    const [h] = storeHealth([{ ...store, bundlePricing: 'FIXED', effectivePrice: null }], [opt([sel('a', listed(500))])])
    expect(h.issues.at(-1)?.level).toBe('destructive')
    expect(h.price).toBeNull()
  })
})
