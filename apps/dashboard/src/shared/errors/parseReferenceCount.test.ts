import { describe, expect, it } from 'vitest'
import { parseReferenceCount } from './parseReferenceCount'

describe('parseReferenceCount', () => {
  it('reads the count from an in-use delete error', () => {
    expect(parseReferenceCount('COUNTRY_IN_USE: 3 references')).toBe(3)
    expect(parseReferenceCount('locale still has 1 reference')).toBe(1)
  })

  it('is case-insensitive', () => {
    expect(parseReferenceCount('12 REFERENCES')).toBe(12)
  })

  it('returns null when the message carries no count', () => {
    expect(parseReferenceCount('FOREIGN KEY violation')).toBeNull()
    expect(parseReferenceCount('')).toBeNull()
  })
})
