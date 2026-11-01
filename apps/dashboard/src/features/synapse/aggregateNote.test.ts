import { describe, expect, it } from 'vitest'

import { aggregateNote } from './aggregateNote'

describe('aggregateNote', () => {
  it('names the share of a total counted as nobody', () => {
    expect(aggregateNote('total', 3)).toBe('includes 3 from visitors who declined analytics')
  })
  it('says nothing for a count of people, which leaves them out, or when there are none', () => {
    expect(aggregateNote('unique', 3)).toBeNull()
    expect(aggregateNote('total', 0)).toBeNull()
  })
})
