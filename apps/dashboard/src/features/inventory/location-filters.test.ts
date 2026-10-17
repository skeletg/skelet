import { describe, expect, it } from 'vitest'
import { locationFiltersForQuery } from './location-filters'
describe('location table server filters', () => {
  it('combines classifications, city and customer capabilities without losing false values', () => {
    expect(locationFiltersForQuery([
      { id: 'type', value: ['WAREHOUSE', 'STORAGE'] }, { id: 'city', value: ' Belgrade ' },
      { id: 'isActive', value: true }, { id: 'storefrontVisible', value: false }, { id: 'pickupEnabled', value: false },
    ])).toEqual({ types: ['WAREHOUSE', 'STORAGE'], city: 'Belgrade', isActive: true, storefrontVisible: false, pickupEnabled: false })
  })
  it('ignores malformed URL filter values', () => {
    expect(locationFiltersForQuery([{ id: 'type', value: ['bad'] }, { id: 'isActive', value: 'true' }])).toEqual({ types: [], city: undefined, isActive: undefined, storefrontVisible: undefined, pickupEnabled: undefined })
  })
})
