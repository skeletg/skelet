import { describe, expect, it } from 'vitest'
import {
  clearedStore,
  productContentChanges,
  productContentDiff,
  productCategoryChanges,
  productListingChanges,
  productListingsSchema,
  productMetadataSchema,
  toProductListingValues,
  toggleCategory,
  type ProductContentValues,
} from './product'

// What these pin: only a description that changed is written (the content writer keeps the rest of
// the row), and a formatting-only edit still shows in the review dialog. Per store, an equal price
// typed differently is no change, and a blank price is refused. Metadata keys are unique and required,
// and assigned categories always keep a primary

const doc = (text: string, format = 0) => ({ root: { children: [{ children: [{ text, format }] }] } })

const before: ProductContentValues = {
  shortDescription: doc('Soft cotton'),
  shortDescriptionText: 'Soft cotton',
  descriptionRichText: doc('A classic tee.'),
  descriptionText: 'A classic tee.',
}

describe('product content', () => {
  it('writes only the description that changed', () => {
    const after = { ...before, descriptionRichText: doc('A better tee.'), descriptionText: 'A better tee.' }
    expect(productContentChanges(before, after)).toEqual({ descriptionRichText: doc('A better tee.') })
    expect(productContentDiff(before, after)).toEqual([expect.objectContaining({ label: 'Description', before: 'A classic tee.', after: 'A better tee.' })])
  })

  it('lists a formatting-only edit', () => {
    const after = { ...before, shortDescription: doc('Soft cotton', 1) }
    expect(productContentDiff(before, after)).toEqual([
      expect.objectContaining({ label: 'Short description', before: 'previous formatting', after: 'updated formatting' }),
    ])
  })
})

describe('per-store listings', () => {
  const before = toProductListingValues(['c1', 'c2'], [{ isPublished: true, price: 1250, channel: { id: 'c1' } }])

  it('writes only the stores that changed, and clears only a store that has a listing', () => {
    const after = { stores: [{ ...before.stores[0], price: '12.5' }, clearedStore('c2')] }
    expect(productListingChanges(before, after, (id) => id === 'c1')).toEqual([])
    const repriced = { stores: [{ ...before.stores[0], price: '9' }, before.stores[1]] }
    expect(productListingChanges(before, repriced, (id) => id === 'c1')).toEqual([{ channelId: 'c1', clear: false, isPublished: true, price: 900 }])
  })

  it('refuses a blank store price instead of saving it as zero', () => {
    const after = { stores: [{ ...before.stores[0], price: '' }, before.stores[1]] }
    expect(productListingsSchema.safeParse(after).success).toBe(false)
  })
})

describe('product metadata', () => {
  const parse = (metadata: { key: string; value: string }[]) => productMetadataSchema.safeParse({ metadata, privateMetadata: [] }).success

  it('refuses a key used twice and a value without a key, and lets a blank row through', () => {
    expect(parse([{ key: 'a', value: '1' }, { key: ' a', value: '2' }])).toBe(false)
    expect(parse([{ key: '', value: 'orphan' }])).toBe(false)
    expect(parse([{ key: 'a', value: '1' }, { key: '', value: '' }])).toBe(true)
  })
})

describe('product categories', () => {
  it('keeps a primary while assigning and unassigning', () => {
    const one = toggleCategory({ assigned: [], primaryId: '' }, 'shirts')
    expect(one).toEqual({ assigned: ['shirts'], primaryId: 'shirts' })
    const two = toggleCategory(one, 'sale')
    expect(two.primaryId).toBe('shirts')
    expect(toggleCategory(two, 'shirts')).toEqual({ assigned: ['sale'], primaryId: 'sale' })
  })

  it('moves the primary to an added category', () => {
    const before = { assigned: ['shirts'], primaryId: 'shirts' }
    expect(productCategoryChanges(before, { assigned: ['sale'], primaryId: 'sale' })).toEqual({ add: ['sale'], remove: ['shirts'], primary: 'sale' })
  })
})
