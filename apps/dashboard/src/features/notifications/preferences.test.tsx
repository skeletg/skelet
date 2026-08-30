import { act, cleanup, render } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  MUTED_CATEGORIES_PREF_KEY,
  TOAST_MIN_SEVERITY_PREF_KEY,
  severityAtLeast,
  useMutedCategories,
  useToastMinSeverity,
} from './preferences'

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

function mount<T>(hook: () => T) {
  let latest!: T
  function Harness({ onReady }: { onReady: (s: T) => void }) {
    onReady(hook())
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

describe('severityAtLeast', () => {
  it('ranks the three severities', () => {
    expect(severityAtLeast('info', 'info')).toBe(true)
    expect(severityAtLeast('info', 'warning')).toBe(false)
    expect(severityAtLeast('warning', 'warning')).toBe(true)
    expect(severityAtLeast('critical', 'warning')).toBe(true)
    expect(severityAtLeast('warning', 'critical')).toBe(false)
  })

  // severity is free text on the wire, and an unknown one must not silence a notification louder
  // than the threshold - it ranks as the floor and shows at the "everything" setting
  it('treats an unknown severity as the floor', () => {
    expect(severityAtLeast('apocalyptic', 'info')).toBe(true)
    expect(severityAtLeast('apocalyptic', 'critical')).toBe(false)
  })
})

describe('useMutedCategories', () => {
  it('mutes and unmutes, keeping no row when nothing is muted', () => {
    const h = mount(useMutedCategories)
    expect(h.state.isMuted('review')).toBe(false)
    act(() => h.state.toggle('review'))
    expect(prefValues.get(MUTED_CATEGORIES_PREF_KEY)).toEqual(['review'])
    act(() => h.state.toggle('review'))
    expect(prefValues.has(MUTED_CATEGORIES_PREF_KEY)).toBe(false)
  })

  it('turns everything back on at once', () => {
    prefValues.set(MUTED_CATEGORIES_PREF_KEY, ['review', 'form'])
    const h = mount(useMutedCategories)
    act(() => h.state.unmuteAll())
    expect(prefValues.has(MUTED_CATEGORIES_PREF_KEY)).toBe(false)
  })
})

describe('useToastMinSeverity', () => {
  it('defaults to everything and stores anything stricter', () => {
    const h = mount(useToastMinSeverity)
    expect(h.state.min).toBe('info')
    act(() => h.state.setMin('critical'))
    expect(prefValues.get(TOAST_MIN_SEVERITY_PREF_KEY)).toBe('critical')
    act(() => h.state.setMin('info'))
    expect(prefValues.has(TOAST_MIN_SEVERITY_PREF_KEY)).toBe(false)
  })

  it('falls back when the stored value is not a severity', () => {
    prefValues.set(TOAST_MIN_SEVERITY_PREF_KEY, 'loud')
    expect(mount(useToastMinSeverity).state.min).toBe('info')
  })
})
