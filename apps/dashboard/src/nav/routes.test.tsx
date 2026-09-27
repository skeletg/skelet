import { cleanup, render } from '@testing-library/react'
import { matchPath } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { allRouterRoutes, getFooterNodes, getNavNodes } from './registry'
import { flattenLeaves } from './types'
import { useVisibleNav } from './use-visible-nav'
import './routes'

vi.mock('@/shared/auth/usePermissions', () => ({
  usePermissions: () => ({ has: () => true, loading: false }),
}))
vi.mock('@/shared/features/useFeatures', () => ({
  useFeatures: () => ({ enabled: () => false, loading: false }),
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
