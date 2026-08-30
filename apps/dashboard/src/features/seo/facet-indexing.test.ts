import { describe, expect, it } from 'vitest'

import {
  combinationEstimate,
  hasSelectableValues,
  indexableValueCount,
  type FacetAttr,
} from './facet-indexing'

const attr = (
  slug: string,
  inputType: string,
  indexableValues: number,
  totalValues = indexableValues,
): FacetAttr => ({
  id: `id-${slug}`,
  slug,
  name: slug,
  inputType,
  isIndexable: true,
  h1Position: 'BEFORE',
  values: Array.from({ length: totalValues }, (_, i) => ({
    id: `${slug}-${i}`,
    name: `${slug} ${i}`,
    slug: `${slug}-${i}`,
    isIndexable: i < indexableValues,
  })),
})

const attrs = [
  attr('brand', 'DROPDOWN', 40),
  attr('color', 'SWATCH', 8, 12),
  attr('year', 'NUMERIC', 0),
]

describe('hasSelectableValues', () => {
  it('is true only for the types that carry AttributeValue rows', () => {
    expect(['DROPDOWN', 'MULTISELECT', 'SWATCH'].every(hasSelectableValues)).toBe(true)
    expect(
      ['PLAIN_TEXT', 'NUMERIC', 'BOOLEAN', 'DATE', 'DATE_TIME', 'REFERENCE'].some(
        hasSelectableValues,
      ),
    ).toBe(false)
  })
})

describe('indexableValueCount', () => {
  it('counts only the opted-in values', () => {
    expect(indexableValueCount(attrs[1])).toBe(8)
  })

  // A scalar's buckets live in the search index, so the dashboard cannot count them -
  // reporting 0 would read as "nothing indexes", which is the opposite of the truth
  it('returns null for a scalar rather than zero', () => {
    expect(indexableValueCount(attrs[2])).toBeNull()
  })
})

describe('combinationEstimate', () => {
  it('multiplies the members indexable value counts', () => {
    expect(combinationEstimate(['brand', 'color'], attrs)).toBe(320)
  })

  it('ignores empty slots and refuses a row with fewer than two facets', () => {
    expect(combinationEstimate(['brand', ''], attrs)).toBeNull()
    expect(combinationEstimate([], attrs)).toBeNull()
  })

  it('is uncountable when any member is scalar', () => {
    expect(combinationEstimate(['brand', 'year'], attrs)).toBeNull()
  })

  it('is uncountable when a member is not a known facet', () => {
    expect(combinationEstimate(['brand', 'ghost'], attrs)).toBeNull()
  })

  it('handles three-facet rows', () => {
    expect(combinationEstimate(['brand', 'color', 'brand'], attrs)).toBe(12800)
  })
})
