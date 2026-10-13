import { describe, expect, it } from 'vitest'
import { channelEditsFrom, rawChannelPrices } from './ChannelSurchargeInputs'

const channels = [
  { id: 'c-usd', name: 'Wheels', currencyCode: 'USD' },
  { id: 'c-eur', name: 'Tyres', currencyCode: 'EUR' },
]

describe('channelEditsFrom', () => {
  it('reports nothing when the inputs round-trip what is stored', () => {
    const prev = [{ channelId: 'c-eur', priceValue: 400 }]
    expect(channelEditsFrom(channels, rawChannelPrices(prev), prev)).toBeUndefined()
  })

  it('sets a typed amount and clears an emptied one', () => {
    const prev = [{ channelId: 'c-eur', priceValue: 400 }]
    expect(channelEditsFrom(channels, { 'c-usd': '550', 'c-eur': ' ' }, prev)).toEqual({ 'c-usd': 550, 'c-eur': null })
  })

  it('ignores an amount that is not a whole number of minor units', () => {
    expect(channelEditsFrom(channels, { 'c-eur': '4.5' }, [])).toBeUndefined()
  })

  it('only looks at the channels it is given', () => {
    expect(channelEditsFrom(channels.slice(0, 1), { 'c-eur': '450' }, [])).toBeUndefined()
  })
})
