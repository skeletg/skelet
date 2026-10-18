import { describe, expect, it } from 'vitest'
import { buildCategoryIndex, formatPath, pathOf, searchCategories, subtreeOf, type CategoryOptionRow } from './category-options'

// What this pins: the flat list becomes a tree at any depth, siblings in position order, and nothing
// is lost to a parent the list doesn't hold or to a parent cycle

const row = (id: string, parentId: string | null, name: string | null, position = 0): CategoryOptionRow => ({
  id,
  parentId,
  slug: id,
  name,
  position,
  isPublished: true,
  productCount: 0,
})

describe('buildCategoryIndex', () => {
  it('reaches past the tree query nesting', () => {
    const chain = Array.from({ length: 7 }, (_, i) => row(`c${i}`, i ? `c${i - 1}` : null, `Level ${i + 1}`))
    const index = buildCategoryIndex(chain)
    expect(pathOf(index, 'c6')).toHaveLength(7)
    expect(formatPath(index, 'c2')).toBe('Level 1 › Level 2 › Level 3')
    expect(index.byId.get('c6')?.depth).toBe(6)
  })

  it('orders parents before children and siblings by position, then slug', () => {
    const index = buildCategoryIndex([
      row('b', null, 'B', 1),
      row('a', null, 'A', 1),
      row('first', null, 'First', 0),
      row('a-kid', 'a', 'Kid'),
    ])
    expect(index.ordered.map((n) => n.id)).toEqual(['first', 'a', 'a-kid', 'b'])
  })

  it('labels by slug when a category has no name, and roots a row whose parent is missing', () => {
    const index = buildCategoryIndex([row('orphan', 'not-listed', null)])
    expect(index.byId.get('orphan')?.depth).toBe(0)
    expect(formatPath(index, 'orphan')).toBe('orphan')
  })

  it('keeps rows caught in a parent cycle', () => {
    const index = buildCategoryIndex([row('x', 'y', 'X'), row('y', 'x', 'Y')])
    expect(index.ordered.map((n) => n.id).sort()).toEqual(['x', 'y'])
  })
})

describe('subtreeOf', () => {
  it('holds the category and everything beneath it', () => {
    const index = buildCategoryIndex([
      row('root', null, 'Root'),
      row('mid', 'root', 'Mid'),
      row('leaf', 'mid', 'Leaf'),
      row('other', null, 'Other'),
    ])
    expect([...subtreeOf(index, 'root')].sort()).toEqual(['leaf', 'mid', 'root'])
    expect([...subtreeOf(index, 'other')]).toEqual(['other'])
  })
})

describe('searchCategories', () => {
  it('matches name or slug, case-insensitive, in tree order', () => {
    const index = buildCategoryIndex([
      row('men', null, 'Men'),
      row('men-shirts', 'men', 'Shirts'),
      row('women', null, 'Women', 1),
      row('women-shirts', 'women', 'Shirts'),
    ])
    const hits = searchCategories(index, 'SHIRT')
    expect(hits.map((n) => formatPath(index, n.id))).toEqual(['Men › Shirts', 'Women › Shirts'])
    expect(searchCategories(index, '   ')).toEqual([])
  })
})
