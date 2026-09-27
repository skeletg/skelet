import { describe, expect, it } from 'vitest'

import { ALL_STORES, currencyForChoice, issueChannelId } from './issue'

const stores = [
  { id: 'us', currencyCode: 'USD' },
  { id: 'eu', currencyCode: 'EUR' },
]

describe('gift card store choice', () => {
  it('sends no channel for every store, and the store id otherwise', () => {
    expect(issueChannelId(ALL_STORES)).toBeNull()
    expect(issueChannelId('eu')).toBe('eu')
  })

  it("switches the card to the picked store's currency, since the backend refuses any other", () => {
    expect(currencyForChoice(stores, 'eu', 'USD')).toBe('EUR')
    expect(currencyForChoice(stores, ALL_STORES, 'USD')).toBe('USD')
  })
})
