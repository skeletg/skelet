import { describe, expect, it } from 'vitest'
import { buildBreadcrumbs, buildDirectoryTree } from './directory-paths'

// What this pins: the folder tree the library page and the picker share - sorted, nested, with a parent row even when
// only its subfolder was listed, and breadcrumbs that start at the library

describe('buildDirectoryTree', () => {
  it('nests folders under their parents in name order', () => {
    const tree = buildDirectoryTree(['shoes/red', 'hats', 'shoes', 'shoes/blue'])
    expect(tree.map((n) => n.path)).toEqual(['hats', 'shoes'])
    expect(tree[1].children.map((n) => [n.name, n.path])).toEqual([
      ['blue', 'shoes/blue'],
      ['red', 'shoes/red'],
    ])
  })

  it('makes a row for a parent that was never listed itself', () => {
    const tree = buildDirectoryTree(['promo/2026/autumn'])
    expect(tree).toEqual([
      { name: 'promo', path: 'promo', children: [{ name: '2026', path: 'promo/2026', children: [{ name: 'autumn', path: 'promo/2026/autumn', children: [] }] }] },
    ])
  })

  it('ignores stray slashes and leaves the input alone', () => {
    const paths = ['b/', '/a//x', 'a']
    const tree = buildDirectoryTree(paths)
    expect(tree.map((n) => n.path)).toEqual(['a', 'b'])
    expect(tree[0].children.map((n) => n.path)).toEqual(['a/x'])
    expect(paths).toEqual(['b/', '/a//x', 'a'])
  })
})

describe('buildBreadcrumbs', () => {
  it('starts at the library and adds each folder down the path', () => {
    expect(buildBreadcrumbs('')).toEqual([{ name: 'Library', path: '' }])
    expect(buildBreadcrumbs('promo/2026')).toEqual([
      { name: 'Library', path: '' },
      { name: 'promo', path: 'promo' },
      { name: '2026', path: 'promo/2026' },
    ])
  })
})
