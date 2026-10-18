import { cleanup, render } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import {
  DEFAULT_APPEARANCE,
  RADII,
  SIDEBAR_TOKENS,
  THEME_TOKENS,
  ThemeProvider,
  defaultCustomTheme,
  sidebarColorsOf,
  sidebarThemeAttrs,
  type CustomTheme,
} from './theme'

const CUSTOM_KEY = 'skelet-dash:custom-theme'
const APPEARANCE_KEY = 'skelet-dash:appearance'

function mountWithCustom(custom: CustomTheme) {
  localStorage.setItem(CUSTOM_KEY, JSON.stringify(custom))
  localStorage.setItem(
    APPEARANCE_KEY,
    JSON.stringify({ mode: 'light', palette: 'custom', radius: 'default', scale: 'default' }),
  )
  render(<ThemeProvider>{null}</ThemeProvider>)
  return document.documentElement.style
}

afterEach(() => {
  cleanup()
  localStorage.clear()
  document.documentElement.removeAttribute('style')
})
// jsdom here has no matchMedia and the real ThemeProvider reads it for the system mode
beforeEach(() => {
  localStorage.clear()
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

describe('sidebarColorsOf', () => {
  it('mirrors the main colours while attached', () => {
    const custom = defaultCustomTheme(false)
    expect(sidebarColorsOf(custom, false)).toBe(custom.colors)
  })

  // sparse on purpose: detaching and changing one row must not blank the other eight
  it('falls back to the main colour for any token the rail does not override', () => {
    const custom: CustomTheme = {
      ...defaultCustomTheme(false),
      sidebarColors: { background: '#101010' },
    }
    const rail = sidebarColorsOf(custom, true)
    expect(rail.background).toBe('#101010')
    expect(rail.primary).toBe(custom.colors.primary)
  })
})

describe('custom theme painting', () => {
  // Root carries the content's colours for both sets - a rail with its own palette overrides them
  // on its own element, and anything of the sidebar's that portals out belongs on the page
  it('paints both sets from the main colours at the root', () => {
    const custom = { ...defaultCustomTheme(false), colors: { ...defaultCustomTheme(false).colors, background: '#aabbcc' } }
    const style = mountWithCustom(custom)
    expect(style.getPropertyValue('--background')).toBe('#aabbcc')
    expect(style.getPropertyValue('--sidebar')).toBe('#aabbcc')
  })

  it('leaves no sidebar variable behind when the palette is not custom', () => {
    localStorage.setItem(
      APPEARANCE_KEY,
      JSON.stringify({ mode: 'light', palette: 'contrast', radius: 'default', scale: 'default' }),
    )
    render(<ThemeProvider>{null}</ThemeProvider>)
    const style = document.documentElement.style
    for (const token of THEME_TOKENS) {
      for (const v of [...token.vars, ...token.sidebarVars]) {
        expect(style.getPropertyValue(v)).toBe('')
      }
    }
  })
})

describe('token model', () => {
  // a var driven by nothing keeps the stylesheet's value, which is how a hand-built light theme
  // ended up with unreadable text on its own muted surfaces
  it('drives every sidebar variable the stylesheet defines', () => {
    const painted = new Set(THEME_TOKENS.flatMap((t) => [...t.vars, ...t.sidebarVars]))
    for (const v of [
      '--sidebar',
      '--sidebar-foreground',
      '--sidebar-primary',
      '--sidebar-primary-foreground',
      '--sidebar-accent',
      '--sidebar-accent-foreground',
      '--sidebar-border',
      '--sidebar-ring',
    ]) {
      expect(painted.has(v)).toBe(true)
    }
  })

  it('offers a rail override for every token that has one', () => {
    expect(SIDEBAR_TOKENS.length).toBeGreaterThan(0)
    for (const token of SIDEBAR_TOKENS) expect(token.sidebarVars.length).toBeGreaterThan(0)
  })
})

describe('the shipped default', () => {
  // what the user asked the app to look like out of the box
  it('is white content as an inset sheet on a dark tuned rail', () => {
    expect(DEFAULT_APPEARANCE.mode).toBe('light')
    expect(DEFAULT_APPEARANCE.palette).toBe('tuned')
    expect(DEFAULT_APPEARANCE.sidebarPalette).toBeNull()
    expect(DEFAULT_APPEARANCE.sidebarMode).toBe('dark')
    expect(DEFAULT_APPEARANCE.frame).toBe('inset')
  })

  it('rounds corners at the medium step', () => {
    expect(DEFAULT_APPEARANCE.radius).toBe('default')
  })

  // the ids are stored in preferences, so each step has to keep its id or every admin who picked
  // it silently moves to a different corner
  it('keeps the 0.625rem step reachable under its own id', () => {
    const medium = RADII.find((r) => r.id === 'default')
    expect(medium?.value).toBe('0.625rem')
    expect(medium?.label).toBe('Medium')
  })

  it('paints the default corner rather than leaving it to the stylesheet', () => {
    localStorage.setItem(
      APPEARANCE_KEY,
      JSON.stringify({ mode: 'light', palette: 'skelet', radius: 'subtle', scale: 'default' }),
    )
    render(<ThemeProvider>{null}</ThemeProvider>)
    expect(document.documentElement.style.getPropertyValue('--radius')).toBe('0.3rem')
  })
})

describe('sidebarThemeAttrs', () => {
  const custom = defaultCustomTheme(false)
  // the Skelet Sharp pairing - a built-in palette on both content and rail
  const SHARP = { ...DEFAULT_APPEARANCE, palette: 'skelet' as const, sidebarPalette: 'skelet' as const }

  it('is null when the rail follows the content, so the markup is untouched', () => {
    expect(sidebarThemeAttrs({ ...SHARP, sidebarPalette: null }, custom)).toBeNull()
  })

  // this is what makes a light page with a dark rail expressible at all
  it('scopes a built-in palette to the rail', () => {
    const attrs = sidebarThemeAttrs(SHARP, custom)
    expect(attrs).toEqual({ attr: 'skelet', dark: true })
  })

  // neutral stores as '' but paints through a real [data-theme='neutral'] block
  it('names the neutral block rather than an empty attribute', () => {
    const attrs = sidebarThemeAttrs({ ...SHARP, sidebarPalette: '' }, custom)
    expect(attrs?.attr).toBe('neutral')
  })

  it('carries custom colours inline, since custom has no CSS block', () => {
    const attrs = sidebarThemeAttrs(
      { ...SHARP, sidebarPalette: 'custom', sidebarMode: 'dark' },
      { ...custom, sidebarColors: { background: '#101010' } },
    )
    expect(attrs?.attr).toBeUndefined()
    expect(attrs?.style?.['--sidebar']).toBe('#101010')
    // the rail's subtree treats the "main" roles as its own too, so a popover inside matches
    expect(attrs?.style?.['--background']).toBe('#101010')
    expect(attrs?.style?.['--primary']).toBe(custom.colors.primary)
  })
})

describe('tuned rail', () => {
  const custom = defaultCustomTheme(false)
  const tuned = { ...DEFAULT_APPEARANCE, palette: 'tuned' as const, sidebarPalette: null }

  // inputs and popovers inside a deep rail on a light page have to read dark too
  it('scopes a deep rail on a light page as dark', () => {
    const attrs = sidebarThemeAttrs({ ...tuned, railTone: 'accent' }, custom, false)
    expect(attrs?.dark).toBe(true)
    expect(attrs?.style?.['--background']).toBe(attrs?.style?.['--sidebar'])
  })

  it('needs no scope when the page variables already describe the rail', () => {
    expect(sidebarThemeAttrs({ ...tuned, railTone: 'match' }, custom, false)).toBeNull()
    expect(sidebarThemeAttrs({ ...tuned, railTone: 'dark' }, custom, true)).toBeNull()
  })
})

describe('rows that name no rail', () => {
  // adopting the split default would repaint half a screen the admin never asked about
  it('leaves the rail following the content', () => {
    localStorage.setItem(
      APPEARANCE_KEY,
      JSON.stringify({ mode: 'dark', palette: 'skelet', radius: 'default', scale: 'default' }),
    )
    render(<ThemeProvider>{null}</ThemeProvider>)
    expect(document.documentElement.dataset.theme).toBe('skelet')
  })
})
