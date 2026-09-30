import { describe, expect, it } from 'vitest'
import { dropZone, isDescendant, type CategorySummary } from './category-tree'

// What these pin: the 25/50/25 split of a tree row into before/on/after, and the cycle guard that
// refuses to nest a category under itself or one of its own descendants

const node = (id: string, children: CategorySummary[] = []): CategorySummary =>
  ({ id, slug: id, urlPath: id, position: 0, level: 0, isPublished: true, name: id, productCount: 0, content: [], children }) as unknown as CategorySummary

describe('dropZone', () => {
  it('splits a row into before, on and after', () => {
    expect(dropZone(100, 100, 40)).toBe('before')
    expect(dropZone(109, 100, 40)).toBe('before')
    expect(dropZone(110, 100, 40)).toBe('on')
    expect(dropZone(130, 100, 40)).toBe('on')
    expect(dropZone(131, 100, 40)).toBe('after')
  })

  it('nests when the row has no height to measure', () => {
    expect(dropZone(5, 0, 0)).toBe('on')
  })
})

describe('isDescendant', () => {
  const tree = [node('a', [node('b', [node('c')])]), node('d')]

  it('treats a node as its own descendant', () => {
    expect(isDescendant(tree, 'a', 'a')).toBe(true)
  })

  it('finds children at any depth', () => {
    expect(isDescendant(tree, 'a', 'b')).toBe(true)
    expect(isDescendant(tree, 'a', 'c')).toBe(true)
  })

  it('lets a node move under a sibling or an ancestor', () => {
    expect(isDescendant(tree, 'a', 'd')).toBe(false)
    expect(isDescendant(tree, 'c', 'a')).toBe(false)
  })
})
