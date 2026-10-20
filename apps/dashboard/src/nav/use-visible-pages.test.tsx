import { cleanup, render } from '@testing-library/react'
import { Navigate } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { __resetRegistryForTests, registerDetailRoute, registerNavNode } from './registry'
import { useVisiblePages } from './use-visible-pages'

// What this pins: ⌘K finds route-only pages too - it used to list only sidebar entries, so 41 pages could
// only be reached by clicking through - under the same permission gate, and without redirects or :id paths
const { held } = vi.hoisted(() => ({ held: [] as string[] }))
vi.mock('@/shared/auth/usePermissions', () => ({ usePermissions: () => ({ has: (s: string) => held.includes(s), loading: false }) }))
vi.mock('@/shared/features/useFeatures', () => ({ useFeatures: () => ({ enabled: () => true, loading: false }) }))

const page = <p>page</p>

beforeEach(() => {
  __resetRegistryForTests()
  registerNavNode({ id: 'content', title: 'Content', children: [{ title: 'CMS', path: '/cms', element: page, permission: 'cms.view' }] })
  registerDetailRoute({ title: 'Tags', path: '/cms/tags', element: page, permission: 'cms.view' })
  registerDetailRoute({ title: 'Page detail', path: '/cms/pages/:id', element: page, permission: 'cms.view' })
  registerDetailRoute({ title: 'Old tags', path: '/cms/old-tags', element: <Navigate to="/cms/tags" />, permission: 'cms.view' })
  registerDetailRoute({ title: 'Inbox', path: '/inbox', element: page, permission: 'notifications.view' })
})
afterEach(() => {
  cleanup()
  held.length = 0
})

function pages() {
  let latest!: ReturnType<typeof useVisiblePages>
  function Harness() {
    latest = useVisiblePages()
    return null
  }
  render(<Harness />)
  return latest.pages
}

describe('useVisiblePages', () => {
  it('lists reachable route-only pages under their nav parent, and nothing else', () => {
    held.push('cms.view')
    expect(pages()).toEqual([
      { path: '/cms', title: 'CMS', context: 'Content' },
      { path: '/cms/tags', title: 'Tags', context: 'Content' },
    ])
  })

  it('leaves out a route-only page the admin may not open', () => {
    held.push('cms.view', 'notifications.view')
    expect(pages().map((p) => p.path)).toContain('/inbox')
    held.length = 0
    cleanup()
    expect(pages()).toEqual([])
  })
})
