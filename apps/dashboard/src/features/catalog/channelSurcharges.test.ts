import { describe, expect, it } from 'vitest'
import { channelEditsFrom, invalidChannelPrices, rawChannelPrices } from './channelSurcharges'

const channels = [
  { id: 'c-usd', name: 'Wheels', currencyCode: 'USD' },
  { id: 'c-eur', name: 'Tyres', currencyCode: 'EUR' },
  { id: 'c-jpy', name: 'Kuruma', currencyCode: 'JPY' },
]

describe('channelEditsFrom', () => {
  it('reports nothing when the inputs round-trip what is stored', () => {
    const prev = [{ channelId: 'c-eur', priceValue: 400 }, { channelId: 'c-jpy', priceValue: 500 }]
    expect(rawChannelPrices(prev, channels)).toEqual({ 'c-eur': '4.00', 'c-jpy': '500' })
    expect(channelEditsFrom(channels, rawChannelPrices(prev, channels), prev)).toBeUndefined()
  })

  it('reads amounts in each channel currency and clears an emptied one', () => {
    const prev = [{ channelId: 'c-eur', priceValue: 400 }]
    expect(channelEditsFrom(channels, { 'c-usd': '5.5', 'c-eur': ' ', 'c-jpy': '700' }, prev)).toEqual({ 'c-usd': 550, 'c-eur': null, 'c-jpy': 700 })
  })

  it('skips an amount finer than the currency allows, and names it', () => {
    const raw = { 'c-eur': '4.505', 'c-jpy': '7.5' }
    expect(channelEditsFrom(channels, raw, [])).toBeUndefined()
    expect(invalidChannelPrices(channels, raw).map((c) => c.id)).toEqual(['c-eur', 'c-jpy'])
  })

  it('only looks at the channels it is given', () => {
    expect(channelEditsFrom(channels.slice(0, 1), { 'c-eur': '4.50' }, [])).toBeUndefined()
  })
})
