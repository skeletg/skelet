export type Vividness = 'mono' | 'muted' | 'balanced' | 'vivid'
export type SurfaceTone = 'neutral' | 'warm' | 'cool' | 'accent'
export type SurfaceTint = 'subtle' | 'strong'
export type DarkBase = 'black' | 'dim' | 'lifted'
export type RailTone = 'match' | 'tinted' | 'accent' | 'dark'

export type Tune = {
  accentHue: number
  vividness: Vividness
  surfaceTone: SurfaceTone
  surfaceTint: SurfaceTint
  darkBase: DarkBase
  railTone: RailTone
}

export const VIVIDNESS: { id: Vividness; label: string }[] = [
  { id: 'mono', label: 'Ink' },
  { id: 'muted', label: 'Muted' },
  { id: 'balanced', label: 'Balanced' },
  { id: 'vivid', label: 'Vivid' },
]
export const SURFACE_TONES: { id: SurfaceTone; label: string }[] = [
  { id: 'neutral', label: 'White' },
  { id: 'warm', label: 'Warm' },
  { id: 'cool', label: 'Cool' },
  { id: 'accent', label: 'Accent tint' },
]
export const SURFACE_TINTS: { id: SurfaceTint; label: string }[] = [
  { id: 'subtle', label: 'Subtle' },
  { id: 'strong', label: 'Strong' },
]
export const DARK_BASES: { id: DarkBase; label: string }[] = [
  { id: 'black', label: 'Black' },
  { id: 'dim', label: 'Dim' },
  { id: 'lifted', label: 'Lifted' },
]
export const RAIL_TONES: { id: RailTone; label: string }[] = [
  { id: 'match', label: 'Match the page' },
  { id: 'tinted', label: 'Tinted' },
  { id: 'accent', label: 'Accent' },
  { id: 'dark', label: 'Dark' },
]

export const ACCENT_SWATCHES: { hue: number; label: string }[] = [
  { hue: 25, label: 'Red' },
  { hue: 50, label: 'Orange' },
  { hue: 85, label: 'Gold' },
  { hue: 130, label: 'Lime' },
  { hue: 155, label: 'Green' },
  { hue: 190, label: 'Teal' },
  { hue: 240, label: 'Blue' },
  { hue: 275, label: 'Indigo' },
  { hue: 305, label: 'Violet' },
  { hue: 350, label: 'Pink' },
]

const CHROMA: Record<Vividness, number> = { mono: 0, muted: 0.07, balanced: 0.13, vivid: 0.2 }
const TONE_HUE: Record<Exclude<SurfaceTone, 'accent'>, number> = { neutral: 0, warm: 70, cool: 250 }
const TINT: Record<SurfaceTint, number> = { subtle: 0.012, strong: 0.026 }
const DARK_L: Record<DarkBase, number> = { black: 0.12, dim: 0.18, lifted: 0.235 }

const ok = (l: number, c: number, h: number) =>
  `oklch(${+l.toFixed(3)} ${+Math.max(c, 0).toFixed(3)} ${Math.round(((h % 360) + 360) % 360)})`

export const TUNED_VARS = [
  'background', 'foreground', 'card', 'card-foreground', 'popover', 'popover-foreground',
  'primary', 'primary-foreground', 'secondary', 'secondary-foreground', 'muted', 'muted-foreground',
  'accent', 'accent-foreground', 'destructive', 'ring', 'border', 'input',
  'chart-1', 'chart-2', 'chart-3', 'chart-4', 'chart-5',
  'sidebar', 'sidebar-foreground', 'sidebar-primary', 'sidebar-primary-foreground',
  'sidebar-accent', 'sidebar-accent-foreground', 'sidebar-border', 'sidebar-ring',
].map((v) => `--${v}`)

type Surface = { h: number; c: number }

function surfaceOf(t: Tune): Surface {
  if (t.surfaceTone === 'neutral') return { h: t.accentHue, c: 0 }
  return { h: t.surfaceTone === 'accent' ? t.accentHue : TONE_HUE[t.surfaceTone], c: TINT[t.surfaceTint] }
}

// Yellows and limes only read as themselves when light, which in turn wants dark text on them
function accentOf(t: Tune, dark: boolean) {
  const c = CHROMA[t.vividness]
  if (c === 0) return { l: dark ? 0.92 : 0.22, c: 0, h: t.accentHue, darkText: dark }
  const bright = t.accentHue >= 70 && t.accentHue <= 135
  const l = dark ? (bright ? 0.86 : 0.74) : bright ? 0.78 : 0.56
  return { l, c, h: t.accentHue, darkText: dark || bright }
}

function charts(a: { c: number; h: number }, l: number): string[] {
  const c = Math.max(a.c, 0.12)
  return [ok(l, c, a.h), ok(l + 0.06, c * 0.8, a.h + 45), ok(l + 0.1, c * 0.7, a.h + 120), ok(l - 0.04, c * 0.75, a.h - 70), ok(l + 0.14, c * 0.6, a.h + 200)]
}

function lightScheme(t: Tune): Record<string, string> {
  const s = surfaceOf(t)
  const a = accentOf(t, false)
  const bg = s.c === 0 ? 1 : t.surfaceTint === 'strong' ? 0.975 : 0.99
  const fg = ok(0.2, Math.min(0.045, s.c * 2 + (s.c ? 0.01 : 0)), s.h)
  const pfg = a.darkText ? ok(0.22, Math.min(0.05, a.c), a.h) : ok(0.99, 0, 0)
  const out: Record<string, string> = {
    background: ok(bg, s.c, s.h), foreground: fg,
    card: ok(Math.min(1, bg + 0.025), s.c * 0.4, s.h), 'card-foreground': fg,
    popover: ok(Math.min(1, bg + 0.025), s.c * 0.4, s.h), 'popover-foreground': fg,
    primary: ok(a.l, a.c, a.h), 'primary-foreground': pfg,
    secondary: ok(bg - 0.045, s.c + 0.008, s.h), 'secondary-foreground': fg,
    muted: ok(bg - 0.045, s.c + 0.006, s.h), 'muted-foreground': ok(0.5, Math.min(0.06, s.c * 2), s.h),
    accent: ok(0.94, Math.max(a.c * 0.28, s.c + 0.006), a.h), 'accent-foreground': ok(0.25, a.c * 0.5, a.h),
    destructive: ok(0.58, 0.22, 27),
    ring: ok(a.l, a.c * 0.8, a.h), border: ok(bg - 0.1, s.c + 0.006, s.h), input: ok(bg - 0.1, s.c + 0.006, s.h),
  }
  charts(a, 0.58).forEach((c, i) => (out[`chart-${i + 1}`] = c))
  return { ...out, ...railOf(t, false, bg, s, a, fg, pfg) }
}

function darkScheme(t: Tune, base?: { l: number; c: number; h: number }): Record<string, string> {
  const s = surfaceOf(t)
  const a = accentOf(t, true)
  const l = base?.l ?? DARK_L[t.darkBase]
  const c = base?.c ?? s.c * 1.6
  const h = base?.h ?? s.h
  const fg = ok(0.96, Math.min(0.015, c * 0.4), h)
  const pfg = a.darkText ? ok(0.18, Math.min(0.04, a.c), a.h) : ok(0.99, 0, 0)
  const out: Record<string, string> = {
    background: ok(l, c, h), foreground: fg,
    card: ok(l + 0.04, c * 1.15, h), 'card-foreground': fg,
    popover: ok(l + 0.04, c * 1.15, h), 'popover-foreground': fg,
    primary: ok(a.l, a.c, a.h), 'primary-foreground': pfg,
    secondary: ok(l + 0.08, c, h), 'secondary-foreground': fg,
    muted: ok(l + 0.07, c, h), 'muted-foreground': ok(0.72, Math.min(0.04, c + 0.01), h),
    accent: ok(l + 0.11, Math.max(a.c * 0.3, c), a.h), 'accent-foreground': ok(0.96, 0.02, a.h),
    destructive: ok(0.68, 0.2, 25),
    ring: ok(a.l, a.c * 0.8, a.h), border: ok(l + 0.12, c, h), input: ok(l + 0.14, c, h),
  }
  charts(a, 0.7).forEach((v, i) => (out[`chart-${i + 1}`] = v))
  if (base) return { ...out, ...railVars(ok(l, c, h), ok(l + 0.08, Math.max(c, a.c * 0.2), h), ok(l + 0.1, c, h), fg, a, pfg) }
  return { ...out, ...railOf(t, true, l, { h, c }, a, fg, pfg) }
}

function railVars(bg: string, hover: string, border: string, fg: string, a: { l: number; c: number; h: number }, pfg: string) {
  return {
    sidebar: bg, 'sidebar-foreground': fg,
    'sidebar-primary': ok(a.l, a.c, a.h), 'sidebar-primary-foreground': pfg,
    'sidebar-accent': hover, 'sidebar-accent-foreground': fg,
    'sidebar-border': border, 'sidebar-ring': ok(a.l, a.c * 0.8, a.h),
  }
}

function deepRailBase(t: Tune): { l: number; c: number; h: number } | null {
  if (t.railTone === 'accent') return { l: 0.28, c: Math.max(0.05, CHROMA[t.vividness] * 0.45), h: t.accentHue }
  if (t.railTone === 'dark') {
    const s = surfaceOf(t)
    return { l: 0.2, c: s.c, h: s.h }
  }
  return null
}

function railOf(t: Tune, dark: boolean, l: number, s: Surface, a: ReturnType<typeof accentOf>, fg: string, pfg: string) {
  if (!dark) {
    const deep = deepRailBase(t)
    if (deep) {
      const da = accentOf(t, true)
      return railVars(ok(deep.l, deep.c, deep.h), ok(deep.l + 0.07, deep.c, deep.h), ok(deep.l + 0.08, deep.c * 0.8, deep.h), ok(0.95, 0.01, deep.h), da, da.darkText ? ok(0.18, 0.04, da.h) : ok(0.99, 0, 0))
    }
    const tinted = t.railTone === 'tinted'
    const rl = tinted ? 0.95 : l - 0.025
    const rc = tinted ? Math.max(0.035, CHROMA[t.vividness] * 0.25) : s.c + 0.006
    const rh = tinted ? t.accentHue : s.h
    return railVars(ok(rl, rc, rh), ok(rl - 0.04, rc + 0.02, rh), ok(rl - 0.08, rc, rh), fg, a, pfg)
  }
  const rail =
    t.railTone === 'accent' ? { l: l + 0.03, c: Math.max(0.05, CHROMA[t.vividness] * 0.35), h: t.accentHue }
    : t.railTone === 'tinted' ? { l: l + 0.03, c: Math.max(0.03, s.c), h: t.accentHue }
    : t.railTone === 'dark' ? { l: Math.max(0.08, l - 0.05), c: s.c, h: s.h }
    : { l: l + 0.015, c: s.c, h: s.h }
  return railVars(ok(rail.l, rail.c, rail.h), ok(rail.l + 0.08, Math.max(rail.c, a.c * 0.2), rail.h), ok(rail.l + 0.1, rail.c, rail.h), fg, a, pfg)
}

export function tunedVars(t: Tune, dark: boolean): Record<string, string> {
  const scheme = dark ? darkScheme(t) : lightScheme(t)
  return Object.fromEntries(Object.entries(scheme).map(([k, v]) => [`--${k}`, v]))
}

// A deep rail on a light page gets its own dark scope, so inputs and popovers inside it read dark
export function tunedRailScope(t: Tune, dark: boolean): Record<string, string> | null {
  if (dark) return null
  const base = deepRailBase(t)
  if (!base) return null
  return Object.fromEntries(Object.entries(darkScheme(t, base)).map(([k, v]) => [`--${k}`, v]))
}

export const DEFAULT_TUNE: Tune = {
  accentHue: 25,
  vividness: 'vivid',
  surfaceTone: 'neutral',
  surfaceTint: 'subtle',
  darkBase: 'dim',
  railTone: 'dark',
}

export const TUNE_KEYS = Object.keys(DEFAULT_TUNE) as (keyof Tune)[]

export function readTune(raw: Partial<Record<keyof Tune, unknown>>): Tune {
  const pick = <T extends string>(v: unknown, opts: { id: T }[], d: T): T => (opts.some((o) => o.id === v) ? (v as T) : d)
  const hue = Number(raw.accentHue)
  return {
    accentHue: Number.isFinite(hue) ? Math.round(((hue % 360) + 360) % 360) : DEFAULT_TUNE.accentHue,
    vividness: pick(raw.vividness, VIVIDNESS, DEFAULT_TUNE.vividness),
    surfaceTone: pick(raw.surfaceTone, SURFACE_TONES, DEFAULT_TUNE.surfaceTone),
    surfaceTint: pick(raw.surfaceTint, SURFACE_TINTS, DEFAULT_TUNE.surfaceTint),
    darkBase: pick(raw.darkBase, DARK_BASES, DEFAULT_TUNE.darkBase),
    railTone: pick(raw.railTone, RAIL_TONES, DEFAULT_TUNE.railTone),
  }
}
