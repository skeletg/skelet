import { useCallback, useEffect, useRef } from 'react'
import { usePreference, usePrefsLoaded } from '@/lib/admin-prefs'
import {
  DEFAULT_APPEARANCE,
  DENSITIES,
  FRAMES,
  PALETTES,
  SIDEBAR_TOKENS,
  TYPEFACES,
  THEME_TOKENS,
  appearanceFromLegacyId,
  defaultCustomTheme,
  presetLook,
  useTheme,
  type Appearance,
  type CustomTheme,
  type ThemePreset,
} from '@/lib/theme'
import { TUNE_KEYS, readTune, type Tune } from '@/lib/palette-recipe'

// Every axis, in the order the signature and comparisons walk them - one list, so adding an axis
// can't leave a writer or the sync comparing a stale subset
const AXES = ['mode', 'palette', 'radius', 'scale', 'typeface', 'density', 'frame', 'sidebarSide', 'reduceMotion', 'sidebarPalette', 'sidebarMode', ...TUNE_KEYS] as const

function sameAppearance(a: Appearance, b: Appearance): boolean {
  return AXES.every((k) => a[k] === b[k])
}

// The split shape a row is written in - never the single-id form
function prefOf(a: Appearance, custom: CustomTheme | undefined): ThemePref {
  return {
    mode: a.mode,
    palette: a.palette,
    radius: a.radius,
    scale: a.scale,
    typeface: a.typeface,
    density: a.density,
    frame: a.frame,
    sidebarSide: a.sidebarSide,
    reduceMotion: a.reduceMotion,
    sidebarPalette: a.sidebarPalette,
    sidebarMode: a.sidebarMode,
    ...Object.fromEntries(TUNE_KEYS.map((k) => [k, a[k]])),
    custom,
  }
}

// Bridges appearance (device-local state, applied to the DOM before auth exists) to the roaming
// per-admin preference store, so it follows the admin across devices
//
// Two rules keep it loop-free: every write goes through a handler that touches BOTH sides at once,
// and the sync below only ever applies pref -> theme, gated on the pref's own signature - so a
// local write can't bounce back off the store and revert the click

export const THEME_PREF_KEY = 'ui:theme'

// One key rather than two: independent debounces could land palette 'custom' before its colours, flashing the default
// custom palette `theme` is the single-id form, still read so a row in that shape keeps working
export type ThemePref = Partial<Tune> & {
  mode?: Appearance['mode']
  palette?: Appearance['palette']
  radius?: Appearance['radius']
  scale?: Appearance['scale']
  typeface?: Appearance['typeface']
  density?: Appearance['density']
  frame?: Appearance['frame']
  sidebarSide?: Appearance['sidebarSide']
  reduceMotion?: boolean
  theme?: string
  custom?: CustomTheme
  // null is a real value here (the rail follows the content), so absent and null differ: absent
  // means a row that named no rail at all, which converts to "follows"
  sidebarPalette?: Appearance['sidebarPalette']
  sidebarMode?: Appearance['sidebarMode']
}

// Covers the detached sidebar too - leaving it out meant a rail-only edit carried no signature
// change, so the sync treated it as already-applied and it never reached the other devices
function customSig(c: CustomTheme): string {
  const main = THEME_TOKENS.map((t) => c.colors[t.id] ?? '').join(',')
  const rail = SIDEBAR_TOKENS.map((t) => c.sidebarColors?.[t.id] ?? '').join(',')
  return `${c.dark ? 'd' : 'l'}:${main}|rail:${rail}`
}

function appearanceOf(pref: ThemePref): Appearance {
  if (pref.mode || pref.palette || pref.radius || pref.scale || pref.typeface || pref.density || pref.frame || pref.sidebarSide || pref.reduceMotion) {
    return {
      mode: pref.mode ?? DEFAULT_APPEARANCE.mode,
      palette: PALETTES.some((p) => p.id === pref.palette) ? pref.palette! : '',
      radius: pref.radius ?? DEFAULT_APPEARANCE.radius,
      scale: pref.scale ?? DEFAULT_APPEARANCE.scale,
      // a row from before these axes existed, or naming one this build doesn't know, reads as the default
      typeface: TYPEFACES.some((t) => t.id === pref.typeface) ? pref.typeface! : DEFAULT_APPEARANCE.typeface,
      density: DENSITIES.some((d) => d.id === pref.density) ? pref.density! : DEFAULT_APPEARANCE.density,
      frame: FRAMES.some((f) => f.id === pref.frame) ? pref.frame! : DEFAULT_APPEARANCE.frame,
      sidebarSide: pref.sidebarSide === 'right' ? 'right' : 'left',
      reduceMotion: !!pref.reduceMotion,
      // absent means the row names no rail, so it follows rather than adopting the split default
      // and repainting half a screen the admin never touched
      sidebarPalette:
        pref.sidebarPalette === undefined || !PALETTES.some((p) => p.id === pref.sidebarPalette) ? null : pref.sidebarPalette!,
      sidebarMode: pref.sidebarMode === 'light' ? 'light' : 'dark',
      ...readTune(pref),
    }
  }
  if (pref.theme) return appearanceFromLegacyId(pref.theme, pref.custom?.dark ?? false)
  return DEFAULT_APPEARANCE
}

function prefSig(pref: ThemePref): string {
  const a = appearanceOf(pref)
  return `${AXES.map((k) => String(a[k] ?? 'follow')).join('|')}|${pref.custom ? customSig(pref.custom) : ''}`
}

function isCustomised(c: CustomTheme): boolean {
  return customSig(c) !== customSig(defaultCustomTheme(c.dark))
}

// The only writer - the theme context applies to the DOM and mirrors to localStorage for the next
// boot, the pref carries it to the other devices
export function useThemePref() {
  const { appearance, setAppearance: apply, custom, setCustom: applyCustom } = useTheme()
  const [, setPref] = usePreference<ThemePref | null>(THEME_PREF_KEY, null)

  const write = useCallback(
    (next: Appearance) => {
      apply(next)
      setPref((prev) => prefOf(next, prev?.custom))
    },
    [apply, setPref],
  )

  const setMode = useCallback(
    (mode: Appearance['mode']) => write({ ...appearance, mode }),
    [appearance, write],
  )
  const setPalette = useCallback(
    (palette: Appearance['palette']) => write({ ...appearance, palette }),
    [appearance, write],
  )
  const setRadius = useCallback(
    (radius: Appearance['radius']) => write({ ...appearance, radius }),
    [appearance, write],
  )
  const setScale = useCallback(
    (scale: Appearance['scale']) => write({ ...appearance, scale }),
    [appearance, write],
  )
  const setTypeface = useCallback(
    (typeface: Appearance['typeface']) => write({ ...appearance, typeface }),
    [appearance, write],
  )
  const setDensity = useCallback(
    (density: Appearance['density']) => write({ ...appearance, density }),
    [appearance, write],
  )
  // Every axis the theme names in one write, so the other devices never see half a theme; text size
  // and motion are the admin's own and survive the switch
  const applyPreset = useCallback(
    (preset: ThemePreset) => write({ ...appearance, ...presetLook(preset) }),
    [appearance, write],
  )
  const setFrame = useCallback(
    (frame: Appearance['frame']) => write({ ...appearance, frame }),
    [appearance, write],
  )
  const setSidebarSide = useCallback(
    (sidebarSide: Appearance['sidebarSide']) => write({ ...appearance, sidebarSide }),
    [appearance, write],
  )
  // Turning any dial means you're tuning, so it selects Tuned and hands the rail to the rail dial
  const setTune = useCallback(
    (patch: Partial<Tune>) => write({ ...appearance, ...patch, palette: 'tuned', sidebarPalette: null }),
    [appearance, write],
  )
  const setReduceMotion = useCallback(
    (reduceMotion: boolean) => write({ ...appearance, reduceMotion }),
    [appearance, write],
  )
  const setSidebarPalette = useCallback(
    (sidebarPalette: Appearance['sidebarPalette']) => write({ ...appearance, sidebarPalette }),
    [appearance, write],
  )
  const setSidebarMode = useCallback(
    (sidebarMode: Appearance['sidebarMode']) => write({ ...appearance, sidebarMode }),
    [appearance, write],
  )
  // Every axis at once, including the custom colours - resetting one at a time would leave the
  // Custom palette selected with the shipped hexes under it, which doesn't look like the default
  const resetAppearance = useCallback(() => {
    applyCustom(defaultCustomTheme(false))
    apply(DEFAULT_APPEARANCE)
    setPref(prefOf(DEFAULT_APPEARANCE, undefined))
  }, [apply, applyCustom, setPref])

  // Both halves in one write - two setters back to back would debounce into two pref rows and the
  // second could land first, flashing the rail through a pairing nobody chose
  const setSidebarTheme = useCallback(
    (sidebarPalette: Appearance['sidebarPalette'], sidebarMode: Appearance['sidebarMode']) =>
      write({ ...appearance, sidebarPalette, sidebarMode }),
    [appearance, write],
  )

  const setCustom = useCallback(
    (next: CustomTheme) => {
      applyCustom(next)
      // editing colours has always implied selecting them, and custom carries its own light/dark.
      // The rail's own palette rides along untouched - omitting it here read as "follows the
      // content" on the way back in, so editing one hex silently re-attached a detached rail
      const nextAppearance: Appearance = {
        ...appearance,
        mode: next.dark ? 'dark' : 'light',
        palette: 'custom',
      }
      apply(nextAppearance)
      setPref(prefOf(nextAppearance, next))
    },
    [appearance, apply, applyCustom, setPref],
  )

  return {
    appearance,
    custom,
    setMode,
    setPalette,
    setRadius,
    setScale,
    setTypeface,
    setDensity,
    applyPreset,
    setFrame,
    setSidebarSide,
    setTune,
    setReduceMotion,
    setSidebarPalette,
    setSidebarMode,
    setSidebarTheme,
    setCustom,
    resetAppearance,
  }
}

// Whether anything about the look has been moved off the shipped default, so the reset can hide
// itself when there's nothing to undo
export function isDefaultAppearance(appearance: Appearance, custom: CustomTheme): boolean {
  return sameAppearance(appearance, DEFAULT_APPEARANCE) && !isCustomised(custom)
}

export function useThemePrefSync() {
  const { appearance, setAppearance, custom, setCustom } = useTheme()
  const [pref, setPref] = usePreference<ThemePref | null>(THEME_PREF_KEY, null)
  const loaded = usePrefsLoaded()
  const applied = useRef<string | null>(null)

  useEffect(() => {
    if (!pref) return
    const sig = prefSig(pref)
    if (sig === applied.current) return // already ours, or already applied
    applied.current = sig
    if (pref.custom && customSig(pref.custom) !== customSig(custom)) setCustom(pref.custom)
    const next = appearanceOf(pref)
    if (!sameAppearance(next, appearance)) setAppearance(next)
  }, [pref, appearance, custom, setAppearance, setCustom])

  // Accounts that predate roaming adopt this device's choice once - gated on the hydrate so it can't mark the key dirty
  // before the server copy lands, and skipped for untouched defaults so a deploy doesn't write a row for every admin
  useEffect(() => {
    if (!loaded || pref) return
    // same predicate the reset button hides itself on - two copies would drift the day an axis lands
    if (isDefaultAppearance(appearance, custom)) return
    setPref(prefOf(appearance, isCustomised(custom) ? custom : undefined))
  }, [loaded, pref, appearance, custom, setPref])
}
