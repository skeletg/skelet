import { useEffect, useReducer } from 'react'
import { act, cleanup, render } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { DEFAULT_PAGE_SIZE, useDefaultPageSize, useTablePrefs } from './useTablePrefs'

// usePreference is mocked as a plain in-memory map so the hook's own logic (merge, routing to
// pref vs local state, reset semantics) is what's under test - not the provider plumbing
const { prefValues, listeners } = vi.hoisted(() => ({
  prefValues: new Map<string, unknown>(),
  listeners: new Set<() => void>(),
}))

vi.mock('@/lib/admin-prefs', () => ({
  usePreference: <T,>(key: string, fallback: T) => {
    const value = prefValues.has(key) ? (prefValues.get(key) as T) : fallback
    const set = (next: T | undefined | ((prev: T) => T | undefined)) => {
      const resolved =
        typeof next === 'function'
          ? (next as (prev: T) => T | undefined)(
              prefValues.has(key) ? (prefValues.get(key) as T) : fallback,
            )
          : next
      if (resolved === undefined) prefValues.delete(key)
      else prefValues.set(key, resolved)
      for (const cb of listeners) cb()
    }
    return [value, set]
  },
}))

function Harness({
  prefsKey,
  onReady,
}: {
  prefsKey?: string
  onReady: (s: ReturnType<typeof useTablePrefs>) => void
}) {
  const [, force] = useReducer((n: number) => n + 1, 0)
  useEffect(() => {
    listeners.add(force)
    return () => void listeners.delete(force)
  }, [force])
  onReady(useTablePrefs(prefsKey, { hiddenByDefault: false }))
  return null
}

afterEach(cleanup)
beforeEach(() => {
  prefValues.clear()
  listeners.clear()
})

function mount(prefsKey?: string) {
  let latest!: ReturnType<typeof useTablePrefs>
  render(<Harness prefsKey={prefsKey} onReady={(s) => (latest = s)} />)
  return {
    get state() {
      return latest
    },
  }
}

describe('useDefaultPageSize', () => {
  function SizeHarness({ onReady }: { onReady: (n: number) => void }) {
    onReady(useDefaultPageSize())
    return null
  }

  function mountSize() {
    let latest = 0
    render(<SizeHarness onReady={(n) => (latest = n)} />)
    return latest
  }

  it('falls back to the shared default', () => {
    expect(mountSize()).toBe(DEFAULT_PAGE_SIZE)
  })

  it('takes the stored size', () => {
    prefValues.set('ui:pageSize', 50)
    expect(mountSize()).toBe(50)
  })

  // a hand-edited pref must not blank a list
  it('ignores nonsense and caps the ceiling', () => {
    prefValues.set('ui:pageSize', 0)
    expect(mountSize()).toBe(DEFAULT_PAGE_SIZE)
    prefValues.set('ui:pageSize', 'lots')
    expect(mountSize()).toBe(DEFAULT_PAGE_SIZE)
    prefValues.set('ui:pageSize', 100000)
    expect(mountSize()).toBe(500)
  })
})

describe('useTablePrefs', () => {
  it('stored visibility overrides the page default column by column', () => {
    prefValues.set('table:orders', { v: { hiddenByDefault: true } })
    const h = mount('orders')
    expect(h.state.columnVisibility).toEqual({ hiddenByDefault: true })
  })

  it('writes layout changes under the table key', () => {
    const h = mount('orders')
    act(() => h.state.setColumnSizing({ name: 240 }))
    act(() => h.state.setDense(true))
    expect(prefValues.get('table:orders')).toMatchObject({ s: { name: 240 }, d: true })
  })

  it('reset clears layout but keeps density - a reading preference, not layout', () => {
    prefValues.set('table:orders', { v: { x: false }, o: ['a'], s: { a: 100 }, d: true })
    const h = mount('orders')
    act(() => h.state.resetLayout())
    expect(prefValues.get('table:orders')).toEqual({ d: true })
  })

  it('reset deletes the key entirely when density was never touched', () => {
    prefValues.set('table:orders', { v: { x: false } })
    const h = mount('orders')
    act(() => h.state.resetLayout())
    expect(prefValues.has('table:orders')).toBe(false)
  })

  it('inherits the account density default when the table has none of its own', () => {
    prefValues.set('ui:density', 'compact')
    const h = mount('orders')
    expect(h.state.dense).toBe(true)
  })

  it('a table that set its own density ignores the account default', () => {
    prefValues.set('ui:density', 'compact')
    prefValues.set('table:orders', { d: false })
    const h = mount('orders')
    expect(h.state.dense).toBe(false)
  })

  it('keyless tables inherit the default too, then honour their own toggle', () => {
    prefValues.set('ui:density', 'compact')
    const h = mount(undefined)
    expect(h.state.dense).toBe(true)
    act(() => h.state.setDense(false))
    expect(h.state.dense).toBe(false)
  })

  it('keyless tables stay ephemeral and never write a pref', () => {
    const h = mount(undefined)
    act(() => h.state.setDense(true))
    act(() => h.state.setColumnOrder(['b', 'a']))
    expect(h.state.dense).toBe(true)
    expect(h.state.columnOrder).toEqual(['b', 'a'])
    expect(prefValues.size).toBe(0)
  })
})
