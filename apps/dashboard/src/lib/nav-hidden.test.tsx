import { act, cleanup, render } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useHiddenGroups } from './nav-hidden'

const { prefValues, listeners } = vi.hoisted(() => ({
  prefValues: new Map<string, unknown>(),
  listeners: new Set<() => void>(),
}))

vi.mock('@/lib/admin-prefs', () => ({
  usePreference: <T,>(key: string, fallback: T) => {
    const read = () => (prefValues.has(key) ? (prefValues.get(key) as T) : fallback)
    return [
      read(),
      (next: T | undefined | ((prev: T) => T | undefined)) => {
        const resolved = typeof next === 'function' ? (next as (p: T) => T | undefined)(read()) : next
        if (resolved === undefined) prefValues.delete(key)
        else prefValues.set(key, resolved)
        for (const cb of listeners) cb()
      },
    ]
  },
}))

function mount() {
  let latest!: ReturnType<typeof useHiddenGroups>
  function Harness({ onReady }: { onReady: (s: ReturnType<typeof useHiddenGroups>) => void }) {
    onReady(useHiddenGroups())
    return null
  }
  render(<Harness onReady={(s) => (latest = s)} />)
  return {
    get state() {
      return latest
    },
  }
}

afterEach(cleanup)
beforeEach(() => {
  prefValues.clear()
  listeners.clear()
})

describe('useHiddenGroups', () => {
  it('starts with everything shown', () => {
    const h = mount()
    expect(h.state.hidden).toEqual([])
    expect(h.state.isHidden('orders')).toBe(false)
  })

  it('hides and unhides a group', () => {
    const h = mount()
    act(() => h.state.toggle('orders'))
    expect(prefValues.get('nav:hiddenGroups')).toEqual(['orders'])
    act(() => h.state.toggle('orders'))
    expect(prefValues.has('nav:hiddenGroups')).toBe(false)
  })

  // an untouched account should carry no row at all, so unhiding the last one deletes the key
  it('drops the key when nothing is hidden any more', () => {
    prefValues.set('nav:hiddenGroups', ['orders', 'catalog'])
    const h = mount()
    act(() => h.state.toggle('orders'))
    expect(prefValues.get('nav:hiddenGroups')).toEqual(['catalog'])
    act(() => h.state.toggle('catalog'))
    expect(prefValues.has('nav:hiddenGroups')).toBe(false)
  })

  it('show all clears every group at once', () => {
    prefValues.set('nav:hiddenGroups', ['orders', 'catalog'])
    const h = mount()
    act(() => h.state.showAll())
    expect(prefValues.has('nav:hiddenGroups')).toBe(false)
  })
})
