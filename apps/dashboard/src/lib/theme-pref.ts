import { useCallback, useEffect, useRef } from 'react'
import { usePreference, usePrefsLoaded } from '@/lib/admin-prefs'
import {
  DEFAULT_APPEARANCE,
  SIDEBAR_TOKENS,
  THEME_TOKENS,
  appearanceFromLegacyId,
  defaultCustomTheme,
  useTheme,
  type Appearance,
  type CustomTheme,
} from '@/lib/theme'

// Bridges appearance (device-local state, applied to the DOM before auth exists) to the roaming
// per-admin preference store, so it follows the admin across devices
//
// Two rules keep it loop-free: every write goes through a handler that touches BOTH sides at once,
// and the sync below only ever applies pref -> theme, gated on the pref's own signature - so a
// local write can't bounce back off the store and revert the click

export const THEME_PREF_KEY = 'ui:theme'

// One key rather than two: independent debounces could land palette 'custom' before its colours, flashing the default
// custom palette `theme` is the single-id form, still read so a row in that shape keeps working
export type ThemePref = {
  mode?: Appearance['mode']
  palette?: Appearance['palette']
  radius?: Appearance['radius']
  scale?: Appearance['scale']
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
  if (pref.mode || pref.palette || pref.radius || pref.scale || pref.reduceMotion) {
    return {
      mode: pref.mode ?? DEFAULT_APPEARANCE.mode,
      palette: pref.palette ?? '',
      radius: pref.radius ?? DEFAULT_APPEARANCE.radius,
      scale: pref.scale ?? DEFAULT_APPEARANCE.scale,
      reduceMotion: !!pref.reduceMotion,
      // absent means the row names no rail, so it follows rather than adopting the split default
      // and repainting half a screen the admin never touched
      sidebarPalette: pref.sidebarPalette === undefined ? null : pref.sidebarPalette,
      sidebarMode: pref.sidebarMode === 'light' ? 'light' : 'dark',
    }
  }
  if (pref.theme) return appearanceFromLegacyId(pref.theme, pref.custom?.dark ?? false)
  return DEFAULT_APPEARANCE
}

function prefSig(pref: ThemePref): string {
  const a = appearanceOf(pref)
  return `${a.mode}|${a.palette}|${a.radius}|${a.scale}|${a.reduceMotion ? 'still' : 'motion'}|${a.sidebarPalette ?? 'follow'}|${a.sidebarMode}|${pref.custom ? customSig(pref.custom) : ''}`
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
      // writes only the split shape, so a row never carries both
      setPref((prev) => ({
        mode: next.mode,
        palette: next.palette,
        radius: next.radius,
        scale: next.scale,
        reduceMotion: next.reduceMotion,
        sidebarPalette: next.sidebarPalette,
        sidebarMode: next.sidebarMode,
        custom: prev?.custom,
      }))
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
    setPref({ ...DEFAULT_APPEARANCE, custom: undefined })
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
      setPref((prev) => ({
        mode: nextAppearance.mode,
        palette: 'custom',
        radius: prev?.radius ?? appearance.radius,
        scale: prev?.scale ?? appearance.scale,
        reduceMotion: prev?.reduceMotion ?? appearance.reduceMotion,
        sidebarPalette: nextAppearance.sidebarPalette,
        sidebarMode: nextAppearance.sidebarMode,
        custom: next,
      }))
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
  return (
    appearance.mode === DEFAULT_APPEARANCE.mode &&
    appearance.palette === DEFAULT_APPEARANCE.palette &&
    appearance.radius === DEFAULT_APPEARANCE.radius &&
    appearance.scale === DEFAULT_APPEARANCE.scale &&
    appearance.reduceMotion === DEFAULT_APPEARANCE.reduceMotion &&
    appearance.sidebarPalette === DEFAULT_APPEARANCE.sidebarPalette &&
    appearance.sidebarMode === DEFAULT_APPEARANCE.sidebarMode &&
    !isCustomised(custom)
  )
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
    if (
      next.mode !== appearance.mode ||
      next.palette !== appearance.palette ||
      next.radius !== appearance.radius ||
      next.scale !== appearance.scale ||
      next.reduceMotion !== appearance.reduceMotion ||
      next.sidebarPalette !== appearance.sidebarPalette ||
      next.sidebarMode !== appearance.sidebarMode
    ) {
      setAppearance(next)
    }
  }, [pref, appearance, custom, setAppearance, setCustom])

  // Accounts that predate roaming adopt this device's choice once - gated on the hydrate so it can't mark the key dirty
  // before the server copy lands, and skipped for untouched defaults so a deploy doesn't write a row for every admin
  useEffect(() => {
    if (!loaded || pref) return
    // same predicate the reset button hides itself on - two copies would drift the day an axis lands
    if (isDefaultAppearance(appearance, custom)) return
    setPref({
      mode: appearance.mode,
      palette: appearance.palette,
      radius: appearance.radius,
      scale: appearance.scale,
      reduceMotion: appearance.reduceMotion,
      sidebarPalette: appearance.sidebarPalette,
      sidebarMode: appearance.sidebarMode,
      custom: isCustomised(custom) ? custom : undefined,
    })
  }, [loaded, pref, appearance, custom, setPref])
}
