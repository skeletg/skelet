import { describe, expect, it } from 'vitest'
import { productFilterVariables } from './product-filters'

describe('productFilterVariables', () => {
  it('leaves every filter off when nothing is chosen', () => {
    expect(productFilterVariables([])).toEqual({
      categoryIds: null, collectionIds: null, priceMin: null, priceMax: null, inStock: null, attributeValueIds: null,
    })
  })

  it('maps each chosen filter to its query variable', () => {
    expect(productFilterVariables([
      { id: 'category', value: ['c1', 'c2'] },
      { id: 'collection', value: ['col1'] },
      { id: 'price', value: { min: 12.5, max: 40 } },
      { id: 'stock', value: ['out'] },
      { id: 'attribute:a1', value: ['value:v1', 'value:v2'] },
      { id: 'attribute:a2', value: ['value:v3'] },
    ])).toEqual({
      categoryIds: ['c1', 'c2'], collectionIds: ['col1'], priceMin: '12.5', priceMax: '40', inStock: false,
      attributeValueIds: ['v1', 'v2', 'v3'],
    })
  })

  it('treats both stock states, a one-sided or negative price and non-value attribute ticks as no filter', () => {
    expect(productFilterVariables([
      { id: 'stock', value: ['in', 'out'] },
      { id: 'price', value: { min: -1 } },
      { id: 'attribute:a1', value: ['yes'] },
    ])).toMatchObject({ inStock: null, priceMin: null, priceMax: null, attributeValueIds: null })
    expect(productFilterVariables([{ id: 'price', value: { max: 9 } }])).toMatchObject({ priceMin: null, priceMax: '9' })
  })
})
