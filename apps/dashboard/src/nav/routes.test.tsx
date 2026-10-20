import { cleanup, render } from '@testing-library/react'
import { matchPath } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { allRouterRoutes, getFooterNodes, getNavNodes } from './registry'
import { flattenLeaves } from './types'
import { useVisibleNav } from './use-visible-nav'
import { useVisiblePages } from './use-visible-pages'
import './routes'

vi.mock('@/shared/auth/usePermissions', () => ({
  usePermissions: () => ({ has: () => true, loading: false }),
}))
const flags = vi.hoisted(() => ({ on: false }))
vi.mock('@/shared/features/useFeatures', () => ({
  useFeatures: () => ({ enabled: () => flags.on, loading: false }),
}))

function visibleSidebarPaths(): string[] {
  let paths: string[] = []
  function Harness() {
    paths = flattenLeaves([
      ...useVisibleNav(getNavNodes()).nodes,
      ...useVisibleNav(getFooterNodes()).nodes,
    ]).map((l) => l.path)
    return null
  }
  render(<Harness />)
  return paths
}

afterEach(cleanup)

describe('the sidebar with the signals feature off', () => {
  const signalsRoutes = allRouterRoutes().filter((r) => r.feature === 'signals').map((r) => r.path)

  it('offers no entry that lands on a signals-only route', () => {
    const leaks = visibleSidebarPaths().filter((p) => signalsRoutes.some((r) => matchPath(r, p)))
    expect(leaks).toEqual([])
  })

  it('offers no list page whose detail routes are signals-only', () => {
    const leaks = visibleSidebarPaths().filter(
      (p) => p !== '/' && signalsRoutes.some((r) => r.startsWith(p + '/')),
    )
    expect(leaks).toEqual([])
  })

  it('still sees signals routes to guard', () => {
    expect(signalsRoutes).toEqual(expect.arrayContaining(['/emails', '/flows', '/segments']))
  })
})

// ⌘K lists route-only pages too, so two with one title read as the same page there - Segments and New form did
describe('page titles', () => {
  it.each([false, true])('names every listable page uniquely, features on: %s', (on) => {
    flags.on = on
    let titles: string[] = []
    function Harness() {
      titles = useVisiblePages().pages.map((p) => p.title)
      return null
    }
    render(<Harness />)
    const seen = new Set<string>()
    expect(titles.filter((t) => seen.size === seen.add(t).size)).toEqual([])
    flags.on = false
  })
})
