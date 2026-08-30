import { useEffect, useReducer } from 'react'
import { act, cleanup, render } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { DEFAULT_APPEARANCE, ThemeProvider, defaultCustomTheme } from './theme'
import { THEME_PREF_KEY, useThemePref, useThemePrefSync, type ThemePref } from './theme-pref'

// each assertion below names only the axes it is about, so a new axis lands here once
const DEFAULTS = DEFAULT_APPEARANCE
// A row that names no rail palette of its own leaves the rail following the content, rather than
// adopting the split default and repainting half a screen
const RAIL_FOLLOWS = { ...DEFAULT_APPEARANCE, sidebarPalette: null }

// The prefs store is faked as a plain map so the bridge's own rules are what's tested: writes touch
// both sides, and the inbound sync only ever applies a CHANGED pref
const { store } = vi.hoisted(() => ({
  store: { values: new Map<string, unknown>(), listeners: new Set<() => void>(), loaded: true, writes: 0 },
}))

vi.mock('@/lib/admin-prefs', () => ({
  usePreference: <T,>(key: string, fallback: T) => {
    const value = store.values.has(key) ? (store.values.get(key) as T) : fallback
    const set = (next: T | undefined | ((prev: T) => T | undefined)) => {
      const resolved =
        typeof next === 'function'
          ? (next as (prev: T) => T | undefined)(
              store.values.has(key) ? (store.values.get(key) as T) : fallback,
            )
          : next
      store.writes += 1
      if (resolved === undefined) store.values.delete(key)
      else store.values.set(key, resolved)
      for (const cb of store.listeners) cb()
    }
    return [value, set]
  },
  usePrefsLoaded: () => store.loaded,
}))

function Harness({ onReady }: { onReady: (s: ReturnType<typeof useThemePref>) => void }) {
  const [, force] = useForce()
  useSubscribe(force)
  useThemePrefSync()
  onReady(useThemePref())
  return null
}

function useForce() {
  return useReducer((n: number) => n + 1, 0)
}
function useSubscribe(force: () => void) {
  useEffect(() => {
    store.listeners.add(force)
    return () => void store.listeners.delete(force)
  }, [force])
}

// jsdom here has no matchMedia and the real ThemeProvider reads it for the system mode
beforeEach(() => {
  window.matchMedia = ((query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addEventListener: () => {},
    removeEventListener: () => {},
    addListener: () => {},
    removeListener: () => {},
    dispatchEvent: () => false,
  })) as unknown as typeof window.matchMedia
})

afterEach(cleanup)
beforeEach(() => {
  store.values.clear()
  store.listeners.clear()
  store.loaded = true
  store.writes = 0
  localStorage.clear()
  document.documentElement.className = ''
  delete document.documentElement.dataset.theme
  delete document.documentElement.dataset.motion
  document.documentElement.style.removeProperty('font-size')
})

function mount() {
  let latest!: ReturnType<typeof useThemePref>
  render(
    <ThemeProvider>
      <Harness onReady={(s) => (latest = s)} />
    </ThemeProvider>,
  )
  return {
    get state() {
      return latest
    },
    pref: () => store.values.get(THEME_PREF_KEY) as ThemePref | undefined,
  }
}

describe('theme roaming bridge', () => {
  it('applies a stored preference on mount', () => {
    store.values.set(THEME_PREF_KEY, { mode: 'dark', palette: 'ocean' })
    const h = mount()
    expect(h.state.appearance).toEqual({ ...RAIL_FOLLOWS, mode: 'dark', palette: 'ocean' })
  })

  it('mode and palette are independent - changing one keeps the other', () => {
    const h = mount()
    act(() => h.state.setPalette('rose'))
    act(() => h.state.setMode('dark'))
    expect(h.state.appearance).toEqual({ ...DEFAULTS, mode: 'dark', palette: 'rose' })
    expect(h.pref()).toMatchObject({ mode: 'dark', palette: 'rose' })
  })

  it('a local pick is not reverted by the sync', () => {
    const h = mount()
    act(() => h.state.setPalette('mint'))
    act(() => {
      for (const cb of store.listeners) cb()
    })
    expect(h.state.appearance.palette).toBe('mint')
  })

  it('adopts a value that arrives from another device', () => {
    const h = mount()
    act(() => h.state.setPalette('rose'))
    act(() => {
      store.values.set(THEME_PREF_KEY, { mode: 'light', palette: 'sky' })
      for (const cb of store.listeners) cb()
    })
    expect(h.state.appearance).toEqual({ ...RAIL_FOLLOWS, mode: 'light', palette: 'sky' })
  })

  it('maps a single-id row onto both axes', () => {
    store.values.set(THEME_PREF_KEY, { theme: 'ocean' })
    const h = mount()
    expect(h.state.appearance).toEqual({ ...RAIL_FOLLOWS, mode: 'dark', palette: 'ocean' })
  })

  it('maps a light palette id onto light mode', () => {
    store.values.set(THEME_PREF_KEY, { theme: 'amber' })
    const h = mount()
    expect(h.state.appearance).toEqual({ ...RAIL_FOLLOWS, mode: 'light', palette: 'amber' })
  })

  it('corner radius is a third independent axis', () => {
    const h = mount()
    act(() => h.state.setPalette('mint'))
    act(() => h.state.setRadius('square'))
    expect(h.state.appearance).toEqual({ ...DEFAULTS, palette: 'mint', radius: 'square' })
    expect(h.pref()).toMatchObject({ palette: 'mint', radius: 'square' })
  })

  it('a row written before the radius axis existed reads as the default', () => {
    store.values.set(THEME_PREF_KEY, { mode: 'dark', palette: 'violet' })
    const h = mount()
    // relative to the default rather than a literal id - the shipped corner has moved once already
    expect(h.state.appearance.radius).toBe(DEFAULTS.radius)
  })

  it('editing colours keeps the chosen radius', () => {
    const h = mount()
    act(() => h.state.setRadius('round'))
    const next = defaultCustomTheme(false)
    act(() => h.state.setCustom(next))
    expect(h.state.appearance.radius).toBe('round')
  })

  it('editing colours selects custom and roams them', () => {
    const h = mount()
    const next = { ...defaultCustomTheme(true), colors: { ...defaultCustomTheme(true).colors, primary: '#123456' } }
    act(() => h.state.setCustom(next))
    expect(h.state.appearance).toEqual({ ...DEFAULTS, mode: 'dark', palette: 'custom' })
    expect(h.pref()).toEqual({ ...DEFAULTS, mode: 'dark', palette: 'custom', custom: next })
  })

  it('text size and reduced motion are axes of their own', () => {
    const h = mount()
    act(() => h.state.setScale('large'))
    act(() => h.state.setReduceMotion(true))
    expect(h.state.appearance).toEqual({ ...DEFAULTS, scale: 'large', reduceMotion: true })
    expect(h.pref()).toMatchObject({ scale: 'large', reduceMotion: true })
  })

  it('a row written before those axes existed reads as the defaults', () => {
    store.values.set(THEME_PREF_KEY, { mode: 'dark', palette: 'violet' })
    const h = mount()
    expect(h.state.appearance.scale).toBe('default')
    expect(h.state.appearance.reduceMotion).toBe(false)
  })

  // the DOM is where these two actually live, so assert on the root element rather than the state
  it('applies text size and stamps the motion attribute on the root', () => {
    const h = mount()
    const root = document.documentElement
    act(() => h.state.setScale('larger'))
    expect(root.style.fontSize).toBe('112.5%')
    act(() => h.state.setReduceMotion(true))
    expect(root.dataset.motion).toBe('reduced')
    // back to the defaults and the stylesheet is authoritative again
    act(() => h.state.setScale('default'))
    act(() => h.state.setReduceMotion(false))
    expect(root.style.fontSize).toBe('')
    expect(root.dataset.motion).toBeUndefined()
  })

  it('does not seed before the roaming copy has loaded', () => {
    store.loaded = false
    localStorage.setItem('skelet-dash:appearance', JSON.stringify({ mode: 'dark', palette: 'ocean' }))
    mount()
    expect(store.writes).toBe(0)
  })

  it('does not seed an untouched default', () => {
    mount()
    expect(store.writes).toBe(0)
    expect(store.values.has(THEME_PREF_KEY)).toBe(false)
  })

  it('seeds once for an account that chose an appearance before roaming existed', () => {
    localStorage.setItem('skelet-dash:theme', 'forest') // single-id key
    const h = mount()
    expect(h.pref()).toEqual({ ...RAIL_FOLLOWS, mode: 'dark', palette: 'forest', custom: undefined })
    expect(store.writes).toBe(1)
  })
})
