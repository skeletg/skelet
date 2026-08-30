import { describe, expect, it } from 'vitest'
import { findActiveLeafPath, parentFromLeaves } from './active-path'

// Shaped like the real registry: a hub, a list under it, and a couple of unrelated pages
const LEAVES = [
  { path: '/', title: 'Dashboard' },
  { path: '/cms', title: 'CMS' },
  { path: '/cms/pages', title: 'Pages' },
  { path: '/cms/blocks', title: 'Blocks' },
  { path: '/orders', title: 'Orders' },
]

describe('findActiveLeafPath', () => {
  it('takes the longest owner, not the first match', () => {
    expect(findActiveLeafPath('/cms/pages/abc', LEAVES.map((l) => l.path))).toBe('/cms/pages')
  })

  // '/cmsomething' must not read as living under '/cms'
  it('only matches on a segment boundary', () => {
    expect(findActiveLeafPath('/cmsomething', ['/cms'])).toBeNull()
  })

  it('root owns only itself', () => {
    expect(findActiveLeafPath('/orders', ['/'])).toBeNull()
    expect(findActiveLeafPath('/', ['/'])).toBe('/')
  })
})

describe('parentFromLeaves', () => {
  it('points a subpage at the page above it', () => {
    expect(parentFromLeaves('/cms/pages/abc', LEAVES)).toEqual({ path: '/cms/pages', title: 'Pages' })
  })

  // a destination is not a subpage - a back arrow on the list itself would be a dead end
  it('gives a top-level page no parent', () => {
    expect(parentFromLeaves('/orders', LEAVES)).toBeNull()
    expect(parentFromLeaves('/', LEAVES)).toBeNull()
  })

  it('stops at the nearest registered page rather than the hub', () => {
    expect(parentFromLeaves('/cms/pages/abc/edit', LEAVES)?.path).toBe('/cms/pages')
  })

  // the reason the rule is "deeper than", not "not itself a page": /cms/pages/new is registered in
  // its own right, and asking whether it IS a page would leave it with no way back
  it('gives a registered page a parent when it still sits under one', () => {
    expect(parentFromLeaves('/cms/pages/new', LEAVES)?.path).toBe('/cms/pages')
    expect(parentFromLeaves('/cms/pages', LEAVES)?.path).toBe('/cms')
  })

  // a route nobody registered has nothing to go back to, so the header stays as it was
  it('returns null for an unregistered route', () => {
    expect(parentFromLeaves('/nowhere/deep', LEAVES)).toBeNull()
  })

  it('falls back to the hub when the intermediate page is not registered', () => {
    expect(parentFromLeaves('/cms/snippets/xyz', LEAVES)).toEqual({ path: '/cms', title: 'CMS' })
  })

  // ':id' is a pattern, not a destination - linking to it would 404
  it('never offers a route pattern as the target', () => {
    const withPattern = [...LEAVES, { path: '/cms/pages/:id', title: 'Page detail' }]
    expect(parentFromLeaves('/cms/pages/abc/edit', withPattern)?.path).toBe('/cms/pages')
  })

  // root is a destination, never a parent, or every page would sprout an arrow to the dashboard
  it('never uses root as a parent', () => {
    expect(parentFromLeaves('/orders', [{ path: '/', title: 'Dashboard' }])).toBeNull()
  })
})
