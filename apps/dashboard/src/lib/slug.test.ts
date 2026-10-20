import { describe, expect, it } from 'vitest'
import { slugify } from './slug'

// What this pins: the one slugify the forms, promotions and categories now share produces what the backend's
// validateSlug accepts - lowercase, digits, underscores and hyphens, with accents folded rather than dropped
describe('slugify', () => {
  it.each([
    ['Free Shipping (DACH)!', 'free-shipping-dach'],
    ['Crème Brûlée', 'creme-brulee'],
    ['snake_case stays', 'snake_case-stays'],
    ['A&B', 'a-b'],
    ['  --Trimmed--  ', 'trimmed'],
  ])('%s -> %s', (input, slug) => {
    expect(slugify(input)).toBe(slug)
  })
})
