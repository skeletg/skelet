import { beforeEach, describe, expect, it } from 'vitest'
import type { NavGroup, NavLeaf } from './types'
import {
  __resetRegistryForTests,
  allRouterRoutes,
  extendGroup,
  getFooterNodes,
  getNavNodes,
  registerDetailRoute,
  registerFooterNode,
  registerNavNode,
} from './registry'

// Plugins register through this, so the guards are the point: two nodes claiming one path would
// leave whichever lost shadowed with no error, and a group with no id cannot be extended or hidden

const leaf = (path: string, over: Partial<NavLeaf> = {}): NavLeaf => ({
  title: path,
  path,
  element: 'page',
  ...over,
})

const group = (id: string, children: NavLeaf[]): NavGroup => ({ id, title: id, children })

beforeEach(() => __resetRegistryForTests())

describe('registering', () => {
  it('takes a bare leaf and a group alike', () => {
    registerNavNode(leaf('/orders'))
    registerNavNode(group('catalog', [leaf('/products')]))
    expect(getNavNodes()).toHaveLength(2)
    expect(allRouterRoutes().map((l) => l.path)).toEqual(['/orders', '/products'])
  })

  it('refuses a path already claimed, wherever it was claimed', () => {
    registerNavNode(leaf('/orders'))
    expect(() => registerNavNode(leaf('/orders'))).toThrow(/duplicate path/)
    expect(() => registerFooterNode(leaf('/orders'))).toThrow(/duplicate path/)
    expect(() => registerDetailRoute(leaf('/orders/:id'))).not.toThrow()
    expect(() => registerDetailRoute(leaf('/orders/:id'))).toThrow(/duplicate path/)
  })

  it('sees a duplicate buried inside a group, in both directions', () => {
    registerNavNode(group('catalog', [leaf('/products')]))
    expect(() => registerNavNode(leaf('/products'))).toThrow(/duplicate path/)

    // and the other way round - the incoming GROUP has to be unpacked before it is checked
    __resetRegistryForTests()
    registerNavNode(leaf('/products'))
    expect(() => registerNavNode(group('catalog', [leaf('/products')]))).toThrow(/duplicate path/)
  })

  // the id is what extendGroup targets and what the hidden-groups preference stores
  it('refuses a group with no id', () => {
    expect(() => registerNavNode({ title: 'Nameless', children: [leaf('/x')] })).toThrow(/needs an id/)
  })

  it('keeps footer nodes out of the sidebar list', () => {
    registerFooterNode(leaf('/pinned'))
    expect(getNavNodes()).toEqual([])
    expect(getFooterNodes()).toHaveLength(1)
  })
})

describe('extendGroup', () => {
  it('appends to a group that uses children', () => {
    registerNavNode(group('catalog', [leaf('/products')]))
    extendGroup('catalog', [leaf('/brands')])
    expect(allRouterRoutes().map((l) => l.path)).toEqual(['/products', '/brands'])
  })

  it('can extend a footer group too', () => {
    registerFooterNode(group('pins', [leaf('/a')]))
    extendGroup('pins', [leaf('/b')])
    expect(allRouterRoutes().map((l) => l.path)).toEqual(['/a', '/b'])
  })

  it('throws rather than silently dropping items for an unknown id', () => {
    expect(() => extendGroup('nope', [leaf('/x')])).toThrow(/not found/)
  })

  it('still refuses a path someone else already claimed', () => {
    registerNavNode(group('catalog', [leaf('/products')]))
    expect(() => extendGroup('catalog', [leaf('/products')])).toThrow(/duplicate path/)
  })

  it('lands in the LAST subgroup when none is named', () => {
    registerNavNode({
      id: 'marketing',
      title: 'Marketing',
      subgroups: [{ label: 'New', items: [leaf('/emails')] }, { label: 'Old', items: [leaf('/templates')] }],
    })
    extendGroup('marketing', [leaf('/blasts')])
    const g = getNavNodes()[0] as NavGroup
    expect(g.subgroups?.[1].items.map((i) => i.path)).toEqual(['/templates', '/blasts'])
    expect(g.subgroups?.[0].items).toHaveLength(1)
  })

  it('finds a named subgroup', () => {
    registerNavNode({
      id: 'marketing',
      title: 'Marketing',
      subgroups: [{ label: 'New', items: [leaf('/emails')] }, { label: 'Old', items: [leaf('/templates')] }],
    })
    extendGroup('marketing', [leaf('/blasts')], { subgroupLabel: 'New' })
    const g = getNavNodes()[0] as NavGroup
    expect(g.subgroups?.[0].items.map((i) => i.path)).toEqual(['/emails', '/blasts'])
  })

  it('creates a subgroup that is not there yet, rather than guessing', () => {
    registerNavNode({ id: 'marketing', title: 'Marketing', subgroups: [{ label: 'New', items: [leaf('/emails')] }] })
    extendGroup('marketing', [leaf('/loyalty')], { subgroupLabel: 'Retention' })
    const g = getNavNodes()[0] as NavGroup
    expect(g.subgroups?.map((s) => s.label)).toEqual(['New', 'Retention'])
    expect(g.subgroups?.[1].items.map((i) => i.path)).toEqual(['/loyalty'])
  })
})

describe('allRouterRoutes', () => {
  it('gathers nav, footer and detail-only routes', () => {
    registerNavNode(group('catalog', [leaf('/products')]))
    registerFooterNode(leaf('/pinned'))
    registerDetailRoute(leaf('/products/:id'))
    expect(allRouterRoutes().map((l) => l.path)).toEqual(['/products', '/pinned', '/products/:id'])
  })

  // an action leaf opens UI in place, so routing to it would render a blank page
  it('leaves out a leaf with no element', () => {
    registerNavNode(leaf('/orders'))
    registerNavNode(leaf('/open-palette', { element: undefined, actionId: 'palette' }))
    expect(allRouterRoutes().map((l) => l.path)).toEqual(['/orders'])
  })
})
