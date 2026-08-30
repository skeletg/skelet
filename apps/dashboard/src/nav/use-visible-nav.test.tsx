import { cleanup, render } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useVisibleNav } from './use-visible-nav'
import { isGroup, type NavGroup, type NavNode } from './types'

// The sidebar, the ⌘K palette and the landing preference all gate through this hook now, so its
// rules are worth pinning down once
const { grants, features } = vi.hoisted(() => ({
  grants: { held: [] as string[], loading: false },
  features: { on: [] as string[], loading: false },
}))

vi.mock('@/shared/auth/usePermissions', () => ({
  usePermissions: () => ({ has: (s: string) => grants.held.includes(s), loading: grants.loading }),
}))
vi.mock('@/shared/features/useFeatures', () => ({
  useFeatures: () => ({ enabled: (f: string) => features.on.includes(f), loading: features.loading }),
}))

const NODES: NavNode[] = [
  { title: 'Dashboard', path: '/' },
  {
    id: 'orders',
    title: 'Orders',
    children: [
      { title: 'All orders', path: '/orders', permission: 'orders.view' },
      { title: 'Carts', path: '/carts', permission: 'orders.view' },
    ],
  },
  {
    id: 'b2b',
    title: 'B2B',
    children: [{ title: 'Companies', path: '/b2b', permission: 'b2b.view', feature: 'b2b' }],
  },
  {
    id: 'email',
    title: 'Email',
    subgroups: [
      { label: 'New', items: [{ title: 'Builder', path: '/emails', feature: 'emails' }] },
      { label: 'Old', items: [{ title: 'Templates', path: '/templates', feature: '!emails' }] },
    ],
  },
]

function mount() {
  let latest!: ReturnType<typeof useVisibleNav>
  function Harness({ onReady }: { onReady: (s: ReturnType<typeof useVisibleNav>) => void }) {
    onReady(useVisibleNav(NODES))
    return null
  }
  render(<Harness onReady={(s) => (latest = s)} />)
  return latest
}

const titles = (nodes: NavNode[]) => nodes.map((n) => n.title)
const childTitles = (nodes: NavNode[], id: string) => {
  const group = nodes.find((n): n is NavGroup => isGroup(n) && n.id === id)
  if (!group) return null
  return (group.subgroups?.flatMap((s) => s.items) ?? group.children ?? []).map((l) => l.title)
}

afterEach(cleanup)
beforeEach(() => {
  grants.held = []
  grants.loading = false
  features.on = []
  features.loading = false
})

describe('useVisibleNav', () => {
  it('drops groups whose every child is gated off', () => {
    const { nodes } = mount()
    expect(titles(nodes)).toEqual(['Dashboard', 'Email'])
  })

  it('keeps a group once one child is granted', () => {
    grants.held = ['orders.view']
    expect(childTitles(mount().nodes, 'orders')).toEqual(['All orders', 'Carts'])
  })

  // a granted page behind an off switch still stays hidden - both gates have to pass
  it('needs the feature switch as well as the grant', () => {
    grants.held = ['b2b.view']
    expect(childTitles(mount().nodes, 'b2b')).toBe(null)
    features.on = ['b2b']
    expect(childTitles(mount().nodes, 'b2b')).toEqual(['Companies'])
  })

  it("a '!' prefix shows the entry only while the switch is off", () => {
    expect(childTitles(mount().nodes, 'email')).toEqual(['Templates'])
    features.on = ['emails']
    expect(childTitles(mount().nodes, 'email')).toEqual(['Builder'])
  })

  it('hides gated entries while the grants are still loading, and says so', () => {
    grants.held = ['orders.view']
    grants.loading = true
    const { nodes, loading } = mount()
    expect(loading).toBe(true)
    expect(childTitles(nodes, 'orders')).toBe(null)
  })
})
