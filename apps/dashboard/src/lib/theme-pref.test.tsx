import { useEffect, useReducer } from 'react'
import { act, cleanup, render } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { DEFAULT_APPEARANCE, THEME_PRESETS, ThemeProvider, defaultCustomTheme, presetLook, presetMatches } from './theme'
import { TUNE_KEYS } from './palette-recipe'
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
  delete document.documentElement.dataset.font
  delete document.documentElement.dataset.density
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
    store.values.set(THEME_PREF_KEY, { mode: 'dark', palette: 'skelet' })
    const h = mount()
    expect(h.state.appearance).toEqual({ ...RAIL_FOLLOWS, mode: 'dark', palette: 'skelet' })
  })

  it('mode and palette are independent - changing one keeps the other', () => {
    const h = mount()
    act(() => h.state.setPalette('contrast'))
    act(() => h.state.setMode('dark'))
    expect(h.state.appearance).toEqual({ ...DEFAULTS, mode: 'dark', palette: 'contrast' })
    expect(h.pref()).toMatchObject({ mode: 'dark', palette: 'contrast' })
  })

  it('a local pick is not reverted by the sync', () => {
    const h = mount()
    act(() => h.state.setPalette('contrast'))
    act(() => {
      for (const cb of store.listeners) cb()
    })
    expect(h.state.appearance.palette).toBe('contrast')
  })

  it('adopts a value that arrives from another device', () => {
    const h = mount()
    act(() => h.state.setPalette('contrast'))
    act(() => {
      store.values.set(THEME_PREF_KEY, { mode: 'light', palette: 'tuned' })
      for (const cb of store.listeners) cb()
    })
    expect(h.state.appearance).toEqual({ ...RAIL_FOLLOWS, mode: 'light', palette: 'tuned' })
  })

  it('maps a single-id row onto both axes', () => {
    store.values.set(THEME_PREF_KEY, { theme: 'skelet' })
    const h = mount()
    expect(h.state.appearance).toEqual({ ...RAIL_FOLLOWS, mode: 'dark', palette: 'skelet' })
  })

  it('maps a light palette id onto light mode', () => {
    store.values.set(THEME_PREF_KEY, { theme: 'contrast' })
    const h = mount()
    expect(h.state.appearance).toEqual({ ...RAIL_FOLLOWS, mode: 'light', palette: 'contrast' })
  })

  it('corner radius is a third independent axis', () => {
    const h = mount()
    act(() => h.state.setPalette('contrast'))
    act(() => h.state.setRadius('square'))
    expect(h.state.appearance).toEqual({ ...DEFAULTS, palette: 'contrast', radius: 'square' })
    expect(h.pref()).toMatchObject({ palette: 'contrast', radius: 'square' })
  })

  it('a row written before the radius axis existed reads as the default', () => {
    store.values.set(THEME_PREF_KEY, { mode: 'dark', palette: 'contrast' })
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

  it('typeface and spacing roam and stamp the root, cleared again at the defaults', () => {
    const h = mount()
    const root = document.documentElement
    act(() => h.state.setTypeface('mono'))
    act(() => h.state.setDensity('compact'))
    expect(h.state.appearance).toEqual({ ...DEFAULTS, typeface: 'mono', density: 'compact' })
    expect(h.pref()).toMatchObject({ typeface: 'mono', density: 'compact' })
    expect(root.dataset.font).toBe('mono')
    expect(root.dataset.density).toBe('compact')
    act(() => h.state.setTypeface('geist'))
    act(() => h.state.setDensity('default'))
    expect(root.dataset.font).toBeUndefined()
    expect(root.dataset.density).toBeUndefined()
  })

  // the sync compares and signs every axis - one that only changed the typeface or spacing must still land
  it('adopts a typeface or spacing change that arrives from another device', () => {
    const h = mount()
    act(() => {
      store.values.set(THEME_PREF_KEY, { ...DEFAULTS, typeface: 'serif', density: 'comfortable' })
      for (const cb of store.listeners) cb()
    })
    expect(h.state.appearance.typeface).toBe('serif')
    expect(h.state.appearance.density).toBe('comfortable')
  })

  it('a row naming a typeface or spacing this build does not know reads as the default', () => {
    store.values.set(THEME_PREF_KEY, { mode: 'dark', palette: 'contrast', typeface: 'comic', density: 'cramped' })
    const h = mount()
    expect(h.state.appearance.typeface).toBe('geist')
    expect(h.state.appearance.density).toBe('default')
  })

  // One write for the whole look - two devices must never see half a theme
  it('a theme writes every axis it names in one go, and keeps text size and motion', () => {
    const terminal = THEME_PRESETS.find((p) => p.id === 'terminal')!
    const h = mount()
    act(() => h.state.setScale('large'))
    act(() => h.state.setReduceMotion(true))
    const before = store.writes
    act(() => h.state.applyPreset(terminal))
    expect(store.writes).toBe(before + 1)
    expect(h.state.appearance).toEqual({ ...DEFAULTS, ...presetLook(terminal), scale: 'large', reduceMotion: true })
    expect(presetMatches(terminal, h.state.appearance)).toBe(true)
    // tuning one axis afterwards leaves the theme unselected
    act(() => h.state.setRadius('round'))
    expect(presetMatches(terminal, h.state.appearance)).toBe(false)
  })

  // a layout theme has to undo itself: picking a plain theme afterwards puts the rail back docked on the left
  it('a layout theme moves the rail, and a theme naming no layout puts it back', () => {
    const cockpit = THEME_PRESETS.find((p) => p.id === 'cockpit')!
    const h = mount()
    act(() => h.state.applyPreset(cockpit))
    expect(h.state.appearance).toMatchObject({ frame: 'floating', sidebarSide: 'right' })
    expect(h.pref()).toMatchObject({ frame: 'floating', sidebarSide: 'right' })
    expect(presetMatches(cockpit, h.state.appearance)).toBe(true)
    act(() => h.state.setSidebarSide('left'))
    expect(presetMatches(cockpit, h.state.appearance)).toBe(false)
    act(() => h.state.applyPreset(THEME_PRESETS.find((p) => p.id === 'terminal')!))
    expect(h.state.appearance).toMatchObject({ frame: 'docked', sidebarSide: 'left' })
  })

  it('adopts a layout change from another device, and an unknown style reads as docked', () => {
    const h = mount()
    act(() => {
      store.values.set(THEME_PREF_KEY, { ...DEFAULTS, frame: 'inset', sidebarSide: 'right' })
      for (const cb of store.listeners) cb()
    })
    expect(h.state.appearance).toMatchObject({ frame: 'inset', sidebarSide: 'right' })
    act(() => {
      store.values.set(THEME_PREF_KEY, { ...DEFAULTS, frame: 'hovering', sidebarSide: 'up' } as unknown as ThemePref)
      for (const cb of store.listeners) cb()
    })
    expect(h.state.appearance).toMatchObject({ frame: 'docked', sidebarSide: 'left' })
  })

  it('no two themes share a palette pairing and layout', () => {
    const keys = THEME_PRESETS.map((p) => {
      const l = presetLook(p)
      return [l.mode, l.palette, l.sidebarPalette, l.frame, l.sidebarSide, ...TUNE_KEYS.map((k) => l[k])].join('|')
    })
    expect(new Set(keys).size).toBe(keys.length)
  })

  it('turning a dial selects Tuned, hands the rail to the rail dial and roams', () => {
    const h = mount()
    act(() => h.state.setTune({ accentHue: 190 }))
    expect(h.state.appearance).toMatchObject({ palette: 'tuned', sidebarPalette: null, accentHue: 190 })
    expect(h.pref()).toMatchObject({ palette: 'tuned', accentHue: 190, railTone: DEFAULTS.railTone })
    const root = document.documentElement
    expect(root.dataset.theme).toBeUndefined()
    expect(root.style.getPropertyValue('--primary')).toMatch(/oklch\(.* 190\)/)
    // leaving Tuned must not leave its inline colours painted over the next base
    expect(root.style.getPropertyValue('--chart-1')).not.toBe('')
    act(() => h.state.setPalette('contrast'))
    expect(root.style.getPropertyValue('--chart-1')).toBe('')
    expect(root.dataset.theme).toBe('contrast')
  })

  it('adopts a dial change from another device, and reads unknown dials and bases as defaults', () => {
    const h = mount()
    act(() => h.state.setTune({}))
    // only a dial differs - the palette already matches, so nothing else could carry the change
    act(() => {
      store.values.set(THEME_PREF_KEY, { ...h.pref(), surfaceTone: 'warm', darkBase: 'black' })
      for (const cb of store.listeners) cb()
    })
    expect(h.state.appearance).toMatchObject({ palette: 'tuned', surfaceTone: 'warm', darkBase: 'black' })
    act(() => {
      store.values.set(THEME_PREF_KEY, { ...DEFAULTS, palette: 'rose', vividness: 'neon', accentHue: 'x' } as unknown as ThemePref)
      for (const cb of store.listeners) cb()
    })
    expect(h.state.appearance).toMatchObject({ palette: '', vividness: DEFAULTS.vividness, accentHue: DEFAULTS.accentHue })
  })

  it('a tuned theme resets dials it does not name', () => {
    const h = mount()
    act(() => h.state.setTune({ surfaceTint: 'strong', darkBase: 'black' }))
    const southpaw = THEME_PRESETS.find((p) => p.id === 'southpaw')!
    act(() => h.state.applyPreset(southpaw))
    expect(h.state.appearance).toMatchObject({ surfaceTint: DEFAULTS.surfaceTint, darkBase: DEFAULTS.darkBase })
    expect(presetMatches(southpaw, h.state.appearance)).toBe(true)
    act(() => h.state.setTune({ accentHue: 20 }))
    expect(presetMatches(southpaw, h.state.appearance)).toBe(false)
  })

  it('the default look is the Skelet theme', () => {
    const skelet = THEME_PRESETS.find((p) => p.id === 'skelet')!
    expect(presetMatches(skelet, DEFAULTS)).toBe(true)
    expect(THEME_PRESETS.filter((p) => presetMatches(p, DEFAULTS))).toHaveLength(1)
  })

  it('a row written before those axes existed reads as the defaults', () => {
    store.values.set(THEME_PREF_KEY, { mode: 'dark', palette: 'contrast' })
    const h = mount()
    expect(h.state.appearance.scale).toBe('default')
    expect(h.state.appearance.reduceMotion).toBe(false)
    expect(h.state.appearance.typeface).toBe('geist')
    expect(h.state.appearance.density).toBe('default')
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
    localStorage.setItem('skelet-dash:appearance', JSON.stringify({ mode: 'dark', palette: 'skelet' }))
    mount()
    expect(store.writes).toBe(0)
  })

  it('does not seed an untouched default', () => {
    mount()
    expect(store.writes).toBe(0)
    expect(store.values.has(THEME_PREF_KEY)).toBe(false)
  })

  it('seeds once for an account that chose an appearance before roaming existed', () => {
    localStorage.setItem('skelet-dash:theme', 'skelet') // single-id key
    const h = mount()
    expect(h.pref()).toEqual({ ...RAIL_FOLLOWS, mode: 'dark', palette: 'skelet', custom: undefined })
    expect(store.writes).toBe(1)
  })
})
