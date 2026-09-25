import { describe, expect, it } from 'vitest'
import { productContentChanges, productContentDiff, type ProductContentValues } from './product'

// What these pin: only a description that changed is written (the content writer keeps the rest of
// the row), and a formatting-only edit still shows in the review dialog

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
