import { act, cleanup, render } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  SIDEBAR_OPEN_PREF_KEY,
  SIDEBAR_WIDTH_DEFAULT,
  SIDEBAR_WIDTH_MAX,
  SIDEBAR_WIDTH_MIN,
  SIDEBAR_WIDTH_PREF_KEY,
  clampSidebarWidth,
  useSidebarPref,
  useSidebarWidthPref,
} from './sidebar-pref'

const { prefValues, listeners } = vi.hoisted(() => ({
  prefValues: new Map<string, unknown>(),
  listeners: new Set<() => void>(),
}))

vi.mock('@/lib/admin-prefs', () => ({
  usePreference: <T,>(key: string, fallback: T) => [
    prefValues.has(key) ? (prefValues.get(key) as T) : fallback,
    (next: T | undefined) => {
      if (next === undefined) prefValues.delete(key)
      else prefValues.set(key, next)
      for (const cb of listeners) cb()
    },
  ],
}))

function mount() {
  let latest!: ReturnType<typeof useSidebarPref>
  function Harness({ onReady }: { onReady: (s: ReturnType<typeof useSidebarPref>) => void }) {
    onReady(useSidebarPref())
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

describe('useSidebarPref', () => {
  it('starts expanded', () => {
    expect(mount().state.open).toBe(true)
  })

  // the whole point: shadcn's cookie was write-only, so a collapse never survived a reload
  it('stores a collapse so it survives a reload', () => {
    const h = mount()
    act(() => h.state.setOpen(false))
    expect(prefValues.get(SIDEBAR_OPEN_PREF_KEY)).toBe(false)
    expect(mount().state.open).toBe(false)
  })

  it('expanding again keeps no row, being the default', () => {
    prefValues.set(SIDEBAR_OPEN_PREF_KEY, false)
    const h = mount()
    act(() => h.state.setOpen(true))
    expect(prefValues.has(SIDEBAR_OPEN_PREF_KEY)).toBe(false)
  })
})

function mountWidth() {
  let latest!: ReturnType<typeof useSidebarWidthPref>
  function Harness() {
    latest = useSidebarWidthPref()
    return null
  }
  render(<Harness />)
  return {
    get state() {
      return latest
    },
  }
}

describe('clampSidebarWidth', () => {
  it('holds the drag inside the usable range', () => {
    expect(clampSidebarWidth(10)).toBe(SIDEBAR_WIDTH_MIN)
    expect(clampSidebarWidth(9999)).toBe(SIDEBAR_WIDTH_MAX)
    expect(clampSidebarWidth(300)).toBe(300)
  })

  // a hand-edited row, or a drag that read NaN off a cancelled pointer, must not collapse the rail
  it('falls back to the default for a value that is not a number', () => {
    expect(clampSidebarWidth(Number.NaN)).toBe(SIDEBAR_WIDTH_DEFAULT)
  })
})

describe('useSidebarWidthPref', () => {
  it('starts at the default width', () => {
    expect(mountWidth().state.width).toBe(SIDEBAR_WIDTH_DEFAULT)
  })

  it('stores a resize so it roams', () => {
    const h = mountWidth()
    act(() => h.state.setWidth(340))
    expect(prefValues.get(SIDEBAR_WIDTH_PREF_KEY)).toBe(340)
    expect(mountWidth().state.width).toBe(340)
  })

  it('clamps an out-of-range write rather than storing it', () => {
    const h = mountWidth()
    act(() => h.state.setWidth(5000))
    expect(prefValues.get(SIDEBAR_WIDTH_PREF_KEY)).toBe(SIDEBAR_WIDTH_MAX)
  })

  it('dragging back to the default keeps no row', () => {
    prefValues.set(SIDEBAR_WIDTH_PREF_KEY, 340)
    const h = mountWidth()
    act(() => h.state.setWidth(SIDEBAR_WIDTH_DEFAULT))
    expect(prefValues.has(SIDEBAR_WIDTH_PREF_KEY)).toBe(false)
  })

  it('reset drops the row', () => {
    prefValues.set(SIDEBAR_WIDTH_PREF_KEY, 340)
    const h = mountWidth()
    act(() => h.state.resetWidth())
    expect(prefValues.has(SIDEBAR_WIDTH_PREF_KEY)).toBe(false)
    expect(mountWidth().state.width).toBe(SIDEBAR_WIDTH_DEFAULT)
  })

  // a stored value outside the range still has to render something usable
  it('clamps a stored value on read', () => {
    prefValues.set(SIDEBAR_WIDTH_PREF_KEY, 9999)
    expect(mountWidth().state.width).toBe(SIDEBAR_WIDTH_MAX)
  })
})
