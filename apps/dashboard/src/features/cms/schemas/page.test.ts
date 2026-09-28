import { describe, expect, it } from 'vitest'
import { changedAttributes } from './page'

// What these pin: relationships keep their order, so a reorder is a change; selections are a set

describe('changedAttributes', () => {
  const before = { byAttribute: { a: ['1', '2'], b: ['x'] } }
  const reordered = { byAttribute: { a: ['2', '1'], b: ['x'] } }

  it('treats a reorder as a change only for ordered lists', () => {
    expect(changedAttributes(before, reordered, true)).toEqual(['a'])
    expect(changedAttributes(before, reordered, false)).toEqual([])
  })
})
