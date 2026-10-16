import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useLayoutEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react'
import {
  CircleIcon,
  ContrastIcon,
  MonitorIcon,
  MoonIcon,
  PaletteIcon,
  SkullIcon,
  SlidersHorizontalIcon,
  SunIcon,
  type LucideIcon,
} from 'lucide-react'
import { DEFAULT_TUNE, TUNED_VARS, TUNE_KEYS, readTune, tunedRailScope, tunedVars, type Tune } from '@/lib/palette-recipe'

// Appearance is two independent choices: the MODE decides light or dark, the PALETTE decides the
// hue family. Every palette is authored for both modes in index.css, so any pairing is valid
export type ThemeMode = 'system' | 'light' | 'dark'
export type PaletteId = '' | 'skelet' | 'contrast' | 'tuned' | 'custom'

// The whole rounding scale is calc()'d off --radius in index.css, so one override reshapes every
// rounded-* utility at once
export type RadiusId = 'square' | 'subtle' | 'default' | 'round'

// Tailwind's spacing and type scales are rem-based, so the root font-size scales the whole UI, not just the text. Percentages rather than
// px, so someone who has already raised their browser's base size keeps that and this stays relative to it
export type ScaleId = 'small' | 'default' | 'large' | 'larger'

// The face the whole UI is set in - Geist is the only bundled font, the rest are each OS's own, so
// none costs a download. 'editorial' keeps Geist for body text and sets headings in a serif
export type TypefaceId = 'geist' | 'system' | 'editorial' | 'serif' | 'mono'

// How tightly everything is packed - Tailwind's whole spacing scale derives from --spacing, so one
// override moves every padding, gap and control height while the text size stays put
export type DensityId = 'compact' | 'default' | 'comfortable'

// How the shell is framed: docked is the flush rail, floating lifts the rail into its own card, inset
// sets the content as a sheet on the rail's colour
export type FrameId = 'docked' | 'floating' | 'inset'
export type SidebarSide = 'left' | 'right'

export type ModeDef = { id: ThemeMode; label: string; icon: LucideIcon }
export type PaletteDef = { id: PaletteId; label: string; icon: LucideIcon }
export type RadiusDef = { id: RadiusId; label: string; value: string }
export type ScaleDef = { id: ScaleId; label: string; value: string }
export type TypefaceDef = { id: TypefaceId; label: string; sample: string }
export type DensityDef = { id: DensityId; label: string; value: string }

// The ids are stored in preferences, so they outlive their labels - 'default' is the 0.625rem
// step labelled Medium, while Subtle is what a new account gets
// eslint-disable-next-line react-refresh/only-export-components
export const RADII: RadiusDef[] = [
  { id: 'square', label: 'Square', value: '0rem' },
  { id: 'subtle', label: 'Subtle', value: '0.3rem' },
  { id: 'default', label: 'Medium', value: '0.625rem' },
  { id: 'round', label: 'Round', value: '1rem' },
]

// eslint-disable-next-line react-refresh/only-export-components
export const SCALES: ScaleDef[] = [
  { id: 'small', label: 'Small', value: '93.75%' },
  { id: 'default', label: 'Default', value: '100%' },
  { id: 'large', label: 'Large', value: '106.25%' },
  { id: 'larger', label: 'Larger', value: '112.5%' },
]

// sample is the stack the picker previews each choice in - the same one index.css applies
// eslint-disable-next-line react-refresh/only-export-components
export const TYPEFACES: TypefaceDef[] = [
  { id: 'geist', label: 'Geist', sample: "'Geist Variable', sans-serif" },
  { id: 'system', label: 'System', sample: 'system-ui, sans-serif' },
  { id: 'editorial', label: 'Editorial', sample: "ui-serif, Charter, 'Iowan Old Style', Georgia, serif" },
  { id: 'serif', label: 'Serif', sample: "ui-serif, Charter, 'Iowan Old Style', Georgia, serif" },
  { id: 'mono', label: 'Mono', sample: "ui-monospace, 'SF Mono', Menlo, Consolas, monospace" },
]

// eslint-disable-next-line react-refresh/only-export-components
export const DENSITIES: DensityDef[] = [
  { id: 'compact', label: 'Compact', value: '0.215rem' },
  { id: 'default', label: 'Default', value: '0.25rem' },
  { id: 'comfortable', label: 'Comfortable', value: '0.285rem' },
]

// eslint-disable-next-line react-refresh/only-export-components
export const FRAMES: { id: FrameId; label: string }[] = [
  { id: 'docked', label: 'Docked' },
  { id: 'floating', label: 'Floating' },
  { id: 'inset', label: 'Inset' },
]

// eslint-disable-next-line react-refresh/only-export-components
export const MODES: ModeDef[] = [
  { id: 'system', label: 'System', icon: MonitorIcon },
  { id: 'light', label: 'Light', icon: SunIcon },
  { id: 'dark', label: 'Dark', icon: MoonIcon },
]

// Add an entry here plus both CSS blocks in index.css and the pickers take it from there
// eslint-disable-next-line react-refresh/only-export-components
export const PALETTES: PaletteDef[] = [
  { id: 'skelet', label: 'Skelet', icon: SkullIcon },
  { id: '', label: 'Neutral', icon: CircleIcon },
  { id: 'contrast', label: 'High contrast', icon: ContrastIcon },
  // computed from the dials in lib/palette-recipe, painted as inline variables like custom
  { id: 'tuned', label: 'Tuned', icon: SlidersHorizontalIcon },
  // inline custom properties rather than a CSS block, so it carries its own light/dark base
  { id: 'custom', label: 'Custom', icon: PaletteIcon },
]

// One picker drives several variables so a handful of choices still produce a coherent theme
//
// `vars` paints the main content, `sidebarVars` the nav rail's own copy of the same role. They are
// listed apart so the sidebar can be detached and driven from a second set of colours - when it
// isn't, both lists get the same value and the split is invisible
export type ThemeToken = {
  id: string
  label: string
  group: TokenGroup
  vars: string[]
  sidebarVars: string[]
  light: string
  dark: string
}

// Colour rows are grouped in the picker - ten flat hex fields read as a wall, four short groups
// read as a theme
export type TokenGroup = 'surface' | 'text' | 'accent' | 'line'

// eslint-disable-next-line react-refresh/only-export-components
export const TOKEN_GROUPS: { id: TokenGroup; label: string }[] = [
  { id: 'surface', label: 'Surfaces' },
  { id: 'text', label: 'Text' },
  { id: 'accent', label: 'Accent' },
  { id: 'line', label: 'Lines' },
]

// eslint-disable-next-line react-refresh/only-export-components
export const THEME_TOKENS: ThemeToken[] = [
  { id: 'background', label: 'Background', group: 'surface', vars: ['--background'], sidebarVars: ['--sidebar'], light: '#ffffff', dark: '#1c1c1c' },
  { id: 'card', label: 'Surface', group: 'surface', vars: ['--card', '--popover'], sidebarVars: [], light: '#ffffff', dark: '#232323' },
  { id: 'muted', label: 'Muted surface', group: 'surface', vars: ['--muted', '--accent', '--secondary'], sidebarVars: ['--sidebar-accent'], light: '#f7f7f7', dark: '#2b2b2b' },
  // --accent-foreground and friends ride along with the body text: they are all "normal text on a subtle surface", and
  // leaving them on the stylesheet's value is what made a hand-built light theme with dark surfaces come out unreadable
  { id: 'foreground', label: 'Text', group: 'text', vars: ['--foreground', '--card-foreground', '--popover-foreground', '--accent-foreground', '--secondary-foreground'], sidebarVars: ['--sidebar-foreground', '--sidebar-accent-foreground'], light: '#252525', dark: '#f5f5f5' },
  { id: 'mutedForeground', label: 'Muted text', group: 'text', vars: ['--muted-foreground'], sidebarVars: [], light: '#8e8e8e', dark: '#a1a1a1' },
  { id: 'primary', label: 'Accent', group: 'accent', vars: ['--primary'], sidebarVars: ['--sidebar-primary'], light: '#343434', dark: '#e5484d' },
  { id: 'primaryForeground', label: 'Accent text', group: 'accent', vars: ['--primary-foreground'], sidebarVars: ['--sidebar-primary-foreground'], light: '#fbfbfb', dark: '#fdfdfd' },
  { id: 'destructive', label: 'Destructive', group: 'accent', vars: ['--destructive'], sidebarVars: [], light: '#e5484d', dark: '#e5484d' },
  { id: 'border', label: 'Border', group: 'line', vars: ['--border', '--input'], sidebarVars: ['--sidebar-border'], light: '#e5e5e5', dark: '#343434' },
  { id: 'ring', label: 'Focus ring', group: 'line', vars: ['--ring'], sidebarVars: ['--sidebar-ring'], light: '#b4b4b4', dark: '#6e6e6e' },
]

// The subset a detached sidebar can actually repaint - the rest have no sidebar counterpart and
// always follow the main colour
// eslint-disable-next-line react-refresh/only-export-components
export const SIDEBAR_TOKENS: ThemeToken[] = THEME_TOKENS.filter((t) => t.sidebarVars.length > 0)

export type CustomTheme = {
  dark: boolean
  colors: Record<string, string>
  // Sparse on purpose - a token with no entry here falls back to its main colour, so giving the rail its own palette and changing one thing
  // doesn't force you to restate the other eight. Only read when the sidebar's palette is 'custom'
  sidebarColors?: Record<string, string>
}

const CUSTOM_KEY = 'skelet-dash:custom-theme'

// eslint-disable-next-line react-refresh/only-export-components
export function defaultCustomTheme(dark: boolean): CustomTheme {
  return { dark, colors: Object.fromEntries(THEME_TOKENS.map((t) => [t.id, dark ? t.dark : t.light])) }
}

// What a custom-palette rail paints with, main colours showing through wherever the rail is silent
// eslint-disable-next-line react-refresh/only-export-components
export function sidebarColorsOf(custom: CustomTheme, detached: boolean): Record<string, string> {
  if (!detached) return custom.colors
  return { ...custom.colors, ...custom.sidebarColors }
}

function readCustomTheme(): CustomTheme {
  try {
    const raw = localStorage.getItem(CUSTOM_KEY)
    if (!raw) return defaultCustomTheme(false)
    const parsed = JSON.parse(raw) as CustomTheme
    return {
      dark: !!parsed.dark,
      colors: { ...defaultCustomTheme(!!parsed.dark).colors, ...parsed.colors },
      sidebarColors: parsed.sidebarColors ?? {},
    }
  } catch {
    return defaultCustomTheme(false) // a hand-edited entry must not blank the app
  }
}

export type Appearance = Tune & {
  mode: ThemeMode
  palette: PaletteId
  radius: RadiusId
  scale: ScaleId
  typeface: TypefaceId
  density: DensityId
  frame: FrameId
  sidebarSide: SidebarSide
  // An explicit choice rather than only the OS query - people run a machine with animations on and
  // still want this one app to sit still
  reduceMotion: boolean
  // null means the rail wears whatever the content wears. Set it and the sidebar gets its own
  // palette scoped to its subtree, which is what makes a light page with a dark rail expressible
  sidebarPalette: PaletteId | null
  // The rail's own base. No 'system' here - pairing a rail against the content is a deliberate
  // choice, and letting the OS flip one half of it defeats the point
  sidebarMode: 'light' | 'dark'
}

const STORAGE_KEY = 'skelet-dash:appearance'
const SINGLE_ID_KEY = 'skelet-dash:theme' // one id for the whole shell, not the mode + palette pair
// The shipped look: white content carrying the Skelet accent, against the Skelet dark rail
// eslint-disable-next-line react-refresh/only-export-components
export const DEFAULT_APPEARANCE: Appearance = {
  mode: 'light',
  palette: 'skelet',
  radius: 'subtle',
  scale: 'default',
  typeface: 'geist',
  density: 'default',
  frame: 'docked',
  sidebarSide: 'left',
  reduceMotion: false,
  sidebarPalette: 'skelet',
  sidebarMode: 'dark',
  ...DEFAULT_TUNE,
}

// A whole look in one click - colour, rail, corners, type and density together. Nothing is stored
// about the theme itself: picking one writes every axis, and the tile shows as active only while
// all of them still match, so tuning one afterwards just leaves it unselected
export type ThemePreset = {
  id: string
  label: string
  description: string
  look: Pick<Appearance, 'mode' | 'palette' | 'radius' | 'typeface' | 'density' | 'sidebarPalette' | 'sidebarMode'> &
    Partial<Pick<Appearance, 'frame' | 'sidebarSide'>> &
    Partial<Tune>
}

// A preset that names no layout means the docked left rail, so switching to it undoes a layout one
// eslint-disable-next-line react-refresh/only-export-components
export function presetLook(preset: ThemePreset): ThemePreset['look'] & Pick<Appearance, 'frame' | 'sidebarSide'> {
  const dials = preset.look.palette === 'tuned' ? DEFAULT_TUNE : {}
  return { frame: 'docked', sidebarSide: 'left', ...dials, ...preset.look }
}

// eslint-disable-next-line react-refresh/only-export-components
export const THEME_PRESETS: ThemePreset[] = [
  {
    id: 'skelet',
    label: 'Skelet',
    description: 'The shipped look - white content against the dark rail',
    look: { mode: 'light', palette: 'skelet', radius: 'subtle', typeface: 'geist', density: 'default', sidebarPalette: 'skelet', sidebarMode: 'dark' },
  },
  {
    id: 'operator',
    label: 'Operator',
    description: 'All grey, square and dense, for long days in big tables',
    look: { mode: 'light', palette: '', radius: 'square', typeface: 'system', density: 'compact', sidebarPalette: null, sidebarMode: 'light' },
  },
  {
    id: 'contrast',
    label: 'High contrast',
    description: 'Black and white with strong borders, easiest to read',
    look: { mode: 'light', palette: 'contrast', radius: 'subtle', typeface: 'system', density: 'default', sidebarPalette: 'contrast', sidebarMode: 'dark' },
  },
  {
    id: 'terminal',
    label: 'Terminal',
    description: 'Green on black in monospace, square and tightly packed',
    look: { mode: 'dark', palette: 'tuned', radius: 'square', typeface: 'mono', density: 'compact', sidebarPalette: null, sidebarMode: 'dark', accentHue: 150, vividness: 'vivid', surfaceTone: 'accent', surfaceTint: 'subtle', darkBase: 'black', railTone: 'dark' },
  },
  {
    id: 'editorial',
    label: 'Editorial',
    description: 'Cream paper, a gold accent, serif headings and room to breathe',
    look: { mode: 'light', palette: 'tuned', radius: 'default', typeface: 'editorial', density: 'comfortable', sidebarPalette: null, sidebarMode: 'dark', accentHue: 85, vividness: 'balanced', surfaceTone: 'warm', surfaceTint: 'strong', railTone: 'tinted' },
  },
  {
    id: 'midnight',
    label: 'Midnight',
    description: 'Cool navy end to end with a cyan accent',
    look: { mode: 'dark', palette: 'tuned', radius: 'default', typeface: 'geist', density: 'default', sidebarPalette: null, sidebarMode: 'dark', accentHue: 210, vividness: 'balanced', surfaceTone: 'cool', surfaceTint: 'strong', darkBase: 'dim', railTone: 'dark' },
  },
  {
    id: 'espresso',
    label: 'Espresso',
    description: 'Lifted warm browns, a muted caramel accent, serif headings',
    look: { mode: 'dark', palette: 'tuned', radius: 'default', typeface: 'editorial', density: 'comfortable', sidebarPalette: null, sidebarMode: 'dark', accentHue: 55, vividness: 'muted', surfaceTone: 'warm', surfaceTint: 'strong', darkBase: 'lifted', railTone: 'dark' },
  },
  {
    id: 'sunset',
    label: 'Sunset',
    description: 'Vivid orange on warm white against a deep orange rail, round',
    look: { mode: 'light', palette: 'tuned', radius: 'round', typeface: 'system', density: 'comfortable', sidebarPalette: null, sidebarMode: 'dark', accentHue: 45, vividness: 'vivid', surfaceTone: 'warm', surfaceTint: 'subtle', railTone: 'accent' },
  },
  {
    id: 'gallery',
    label: 'Gallery',
    description: 'Ink on white, content framed as a rounded sheet on a dark rail',
    look: { mode: 'light', palette: 'tuned', radius: 'round', typeface: 'system', density: 'comfortable', sidebarPalette: null, sidebarMode: 'dark', vividness: 'mono', surfaceTone: 'neutral', railTone: 'dark', frame: 'inset' },
  },
  {
    id: 'cockpit',
    label: 'Cockpit',
    description: 'Electric indigo on black, mono and dense, navigation floating on the right',
    look: { mode: 'dark', palette: 'tuned', radius: 'subtle', typeface: 'mono', density: 'compact', sidebarPalette: null, sidebarMode: 'dark', accentHue: 275, vividness: 'vivid', surfaceTone: 'accent', surfaceTint: 'strong', darkBase: 'black', railTone: 'accent', frame: 'floating', sidebarSide: 'right' },
  },
  {
    id: 'atelier',
    label: 'Atelier',
    description: 'Blush pink, a floating tinted rail, serif headings, soft corners',
    look: { mode: 'light', palette: 'tuned', radius: 'round', typeface: 'editorial', density: 'comfortable', sidebarPalette: null, sidebarMode: 'dark', accentHue: 350, vividness: 'balanced', surfaceTone: 'accent', surfaceTint: 'subtle', railTone: 'tinted', frame: 'floating' },
  },
  {
    id: 'southpaw',
    label: 'Southpaw',
    description: 'Green on plain white, with the navigation on the right',
    look: { mode: 'light', palette: 'tuned', radius: 'default', typeface: 'geist', density: 'default', sidebarPalette: null, sidebarMode: 'dark', accentHue: 155, vividness: 'balanced', surfaceTone: 'neutral', railTone: 'match', sidebarSide: 'right' },
  },
]

// eslint-disable-next-line react-refresh/only-export-components
export function presetMatches(preset: ThemePreset, appearance: Appearance): boolean {
  const l = presetLook(preset)
  // the dials only mean anything to a tuned theme
  if (l.palette === 'tuned' && TUNE_KEYS.some((k) => l[k] !== appearance[k])) return false
  return (
    l.frame === appearance.frame &&
    l.sidebarSide === appearance.sidebarSide &&
    l.mode === appearance.mode &&
    l.palette === appearance.palette &&
    l.radius === appearance.radius &&
    l.typeface === appearance.typeface &&
    l.density === appearance.density &&
    l.sidebarPalette === appearance.sidebarPalette &&
    // a rail that follows the content has no mode of its own worth comparing
    (l.sidebarPalette === null || l.sidebarMode === appearance.sidebarMode)
  )
}

// A palette id implies its mode, so a single id decides both halves
const DARK_PALETTES = ['skelet']

// A single id names one theme for the whole shell, so it converts with the rail following the
// content - inheriting the split default would repaint half a screen someone never asked about
// eslint-disable-next-line react-refresh/only-export-components
export function appearanceFromLegacyId(id: string, customDark = false): Appearance {
  const base: Appearance = { ...DEFAULT_APPEARANCE, sidebarPalette: null }
  if (id === 'light' || id === 'dark' || id === 'system') return { ...base, palette: '', mode: id }
  if (id === 'custom') return { ...base, mode: customDark ? 'dark' : 'light', palette: 'custom' }
  if (DARK_PALETTES.includes(id)) return { ...base, mode: 'dark', palette: id as PaletteId }
  if (PALETTES.some((p) => p.id === id)) return { ...base, mode: 'light', palette: id as PaletteId }
  return DEFAULT_APPEARANCE
}

// What the sidebar element has to wear for its own palette to win inside its subtree
//
// Scoped rather than computed onto :root: the palettes already exist in index.css as
// [data-theme='x'] blocks that set every variable, so handing the rail its own attribute reuses
// them wholesale. Custom has no block, so it carries its variables inline instead
export type SidebarThemeAttrs = { attr?: string; dark: boolean; style?: Record<string, string> }

// eslint-disable-next-line react-refresh/only-export-components
export function sidebarThemeAttrs(
  appearance: Appearance,
  custom: CustomTheme,
  pageDark = false,
): SidebarThemeAttrs | null {
  const palette = appearance.sidebarPalette
  if (palette === null && appearance.palette === 'tuned') {
    const scope = tunedRailScope(appearance, pageDark)
    return scope ? { dark: true, style: scope } : null
  }
  if (palette === null) return null // follows the content, nothing to scope
  const dark = appearance.sidebarMode === 'dark'
  if (palette !== 'custom') return { attr: palette || 'neutral', dark }

  const colors = sidebarColorsOf(custom, true)
  const style: Record<string, string> = {}
  for (const token of THEME_TOKENS) {
    const value = colors[token.id]
    if (!value) continue
    // both lists: inside the rail's subtree the "main" roles are the rail's too, so a popover or
    // button rendered in there reads the same theme as the nav around it
    for (const v of token.vars) style[v] = value
    for (const v of token.sidebarVars) style[v] = value
  }
  return { dark, style }
}

function applyTheme(appearance: Appearance, prefersDark: boolean, custom: CustomTheme) {
  const root = document.documentElement
  const isCustom = appearance.palette === 'custom'
  // custom carries its own base, so its switch decides the mode for that palette
  const isDark = isCustom ? custom.dark : appearance.mode === 'system' ? prefersDark : appearance.mode === 'dark'
  root.classList.toggle('dark', isDark)
  if (appearance.palette && !isCustom && appearance.palette !== 'tuned') root.dataset.theme = appearance.palette
  else delete root.dataset.theme

  const radius = RADII.find((r) => r.id === appearance.radius)
  if (radius && radius.id !== 'default') root.style.setProperty('--radius', radius.value)
  else root.style.removeProperty('--radius')

  // Cleared at the default so the stylesheet stays the authority on both of these
  const scale = SCALES.find((sc) => sc.id === appearance.scale)
  if (scale && scale.id !== 'default') root.style.fontSize = scale.value
  else root.style.removeProperty('font-size')

  if (appearance.reduceMotion) root.dataset.motion = 'reduced'
  else delete root.dataset.motion

  // Absent at the defaults, same as radius and scale, so the stylesheet stays the authority
  if (appearance.typeface !== 'geist') root.dataset.font = appearance.typeface
  else delete root.dataset.font
  if (appearance.density !== 'default') root.dataset.density = appearance.density
  else delete root.dataset.density

  // Always cleared first - a stale inline value would otherwise outlive a switch back to a built-in
  for (const token of THEME_TOKENS) {
    for (const v of token.vars) root.style.removeProperty(v)
    for (const v of token.sidebarVars) root.style.removeProperty(v)
  }
  for (const v of TUNED_VARS) root.style.removeProperty(v)
  if (appearance.palette === 'tuned') {
    for (const [v, value] of Object.entries(tunedVars(appearance, isDark))) root.style.setProperty(v, value)
    return
  }
  if (!isCustom) return
  // Root carries the content's colours for BOTH sets. A rail with its own palette overrides them on its own element, and anything of the
  // sidebar's that portals out (a nav dropdown) lands on the page, where matching the content is the right answer
  for (const token of THEME_TOKENS) {
    const value = custom.colors[token.id]
    if (!value) continue
    for (const v of token.vars) root.style.setProperty(v, value)
    for (const v of token.sidebarVars) root.style.setProperty(v, value)
  }
}

function readStoredAppearance(): Appearance {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (raw) {
      const parsed = JSON.parse(raw) as Appearance
      const mode = MODES.some((m) => m.id === parsed.mode) ? parsed.mode : DEFAULT_APPEARANCE.mode
      const palette = PALETTES.some((p) => p.id === parsed.palette) ? parsed.palette : ''
      const radius = RADII.some((r) => r.id === parsed.radius) ? parsed.radius : DEFAULT_APPEARANCE.radius
      const scale = SCALES.some((sc) => sc.id === parsed.scale) ? parsed.scale : DEFAULT_APPEARANCE.scale
      const typeface = TYPEFACES.some((t) => t.id === parsed.typeface) ? parsed.typeface : DEFAULT_APPEARANCE.typeface
      const density = DENSITIES.some((d) => d.id === parsed.density) ? parsed.density : DEFAULT_APPEARANCE.density
      // A row with no sidebar keys at all named one theme for the whole shell, so the rail follows
      // the content rather than adopting the split default
      const stored = parsed as Partial<Appearance>
      const sidebarPalette =
        stored.sidebarPalette === undefined
          ? null
          : stored.sidebarPalette !== null && PALETTES.some((p) => p.id === stored.sidebarPalette)
            ? stored.sidebarPalette
            : null
      return {
        mode,
        palette,
        radius,
        scale,
        typeface,
        density,
        frame: FRAMES.some((f) => f.id === parsed.frame) ? parsed.frame : DEFAULT_APPEARANCE.frame,
        sidebarSide: parsed.sidebarSide === 'right' ? 'right' : 'left',
        reduceMotion: !!parsed.reduceMotion,
        sidebarPalette,
        sidebarMode: stored.sidebarMode === 'light' ? 'light' : 'dark',
        ...readTune(stored),
      }
    }
    const singleId = localStorage.getItem(SINGLE_ID_KEY)
    if (singleId) return appearanceFromLegacyId(singleId, readCustomTheme().dark)
  } catch {
    /* a hand-edited entry must not blank the app */
  }
  return DEFAULT_APPEARANCE
}

type ThemeContextValue = {
  appearance: Appearance
  setAppearance: (next: Appearance) => void
  custom: CustomTheme
  setCustom: (next: CustomTheme) => void
  // what the page actually renders as once System and Custom have had their say
  resolvedDark: boolean
}

const ThemeContext = createContext<ThemeContextValue | null>(null)

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [appearance, setAppearanceState] = useState<Appearance>(() => readStoredAppearance())
  const [custom, setCustomState] = useState<CustomTheme>(() => readCustomTheme())

  const [prefersDark, setPrefersDark] = useState(() => window.matchMedia('(prefers-color-scheme: dark)').matches)

  useLayoutEffect(() => {
    applyTheme(appearance, prefersDark, custom)
  }, [appearance, prefersDark, custom])

  useEffect(() => {
    const media = window.matchMedia('(prefers-color-scheme: dark)')
    const onChange = () => setPrefersDark(media.matches)
    media.addEventListener('change', onChange)
    return () => media.removeEventListener('change', onChange)
  }, [])

  const resolvedDark =
    appearance.palette === 'custom' ? custom.dark : appearance.mode === 'system' ? prefersDark : appearance.mode === 'dark'

  const setAppearance = useCallback((next: Appearance) => {
    setAppearanceState(next)
    localStorage.setItem(STORAGE_KEY, JSON.stringify(next))
  }, [])

  const setCustom = useCallback((next: CustomTheme) => {
    setCustomState(next)
    localStorage.setItem(CUSTOM_KEY, JSON.stringify(next))
  }, [])

  const value = useMemo(
    () => ({ appearance, setAppearance, custom, setCustom, resolvedDark }),
    [appearance, setAppearance, custom, setCustom, resolvedDark],
  )
  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>
}

// eslint-disable-next-line react-refresh/only-export-components
export function useTheme() {
  const ctx = useContext(ThemeContext)
  if (!ctx) throw new Error('useTheme must be used within <ThemeProvider>')
  return ctx
}
