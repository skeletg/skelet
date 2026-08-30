import { cleanup, render } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { DEFAULT_LANDING, useLandingPref } from './landing'

// The store and the nav are both stubbed - what's under test is which path wins, not the plumbing
const { prefValues, pages } = vi.hoisted(() => ({
  prefValues: new Map<string, unknown>(),
  pages: [] as Array<{ path: string; title: string; context?: string }>,
}))

vi.mock('@/lib/admin-prefs', () => ({
  usePreference: <T,>(key: string, fallback: T) => [
    prefValues.has(key) ? (prefValues.get(key) as T) : fallback,
    (next: T | undefined) => {
      if (next === undefined) prefValues.delete(key)
      else prefValues.set(key, next)
    },
  ],
}))

vi.mock('@/nav/use-visible-pages', () => ({
  useVisiblePages: () => ({ pages, loading: false }),
}))

function mount() {
  let latest!: ReturnType<typeof useLandingPref>
  function Harness() {
    latest = useLandingPref()
    return null
  }
  render(<Harness />)
  return latest
}

afterEach(cleanup)
beforeEach(() => {
  prefValues.clear()
  pages.length = 0
  pages.push({ path: '/orders', title: 'Orders', context: 'Sales' })
})

describe('useLandingPref', () => {
  it('defaults to the dashboard', () => {
    const p = mount()
    expect(p.landing).toBe(DEFAULT_LANDING)
    expect(p.title).toBe('Dashboard')
  })

  it('keeps a stored page that is still reachable', () => {
    prefValues.set('ui:landing', '/orders')
    const p = mount()
    expect(p.landing).toBe('/orders')
    expect(p.title).toBe('Orders')
  })

  // a revoked permission or a renamed route must not dead-end every sign-in
  it('falls back to the dashboard when the stored page is gone', () => {
    prefValues.set('ui:landing', '/orders')
    pages.length = 0
    expect(mount().landing).toBe(DEFAULT_LANDING)
  })

  it('choosing the dashboard deletes the key rather than storing the default', () => {
    prefValues.set('ui:landing', '/orders')
    mount().setLanding(DEFAULT_LANDING)
    expect(prefValues.has('ui:landing')).toBe(false)
  })
})
