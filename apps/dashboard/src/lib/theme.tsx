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
  DropletIcon,
  FlameIcon,
  FlowerIcon,
  LeafIcon,
  MonitorIcon,
  MoonIcon,
  PaletteIcon,
  SkullIcon,
  SparklesIcon,
  SunIcon,
  WavesIcon,
  type LucideIcon,
} from 'lucide-react'

// Appearance is two independent choices: the MODE decides light or dark, the PALETTE decides the
// hue family. Every palette is authored for both modes in index.css, so any pairing is valid
export type ThemeMode = 'system' | 'light' | 'dark'
export type PaletteId = '' | 'rose' | 'amber' | 'mint' | 'sky' | 'ocean' | 'forest' | 'violet' | 'crimson' | 'skelet' | 'custom'

// The whole rounding scale is calc()'d off --radius in index.css, so one override reshapes every
// rounded-* utility at once
export type RadiusId = 'square' | 'subtle' | 'default' | 'round'

// Tailwind's spacing and type scales are rem-based, so the root font-size scales the whole UI, not just the text. Percentages rather than
// px, so someone who has already raised their browser's base size keeps that and this stays relative to it
export type ScaleId = 'small' | 'default' | 'large' | 'larger'

export type ModeDef = { id: ThemeMode; label: string; icon: LucideIcon }
export type PaletteDef = { id: PaletteId; label: string; icon: LucideIcon }
export type RadiusDef = { id: RadiusId; label: string; value: string }
export type ScaleDef = { id: ScaleId; label: string; value: string }

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

// eslint-disable-next-line react-refresh/only-export-components
export const MODES: ModeDef[] = [
  { id: 'system', label: 'System', icon: MonitorIcon },
  { id: 'light', label: 'Light', icon: SunIcon },
  { id: 'dark', label: 'Dark', icon: MoonIcon },
]

// Add an entry here plus both CSS blocks in index.css and the pickers take it from there
// eslint-disable-next-line react-refresh/only-export-components
export const PALETTES: PaletteDef[] = [
  { id: '', label: 'Neutral', icon: CircleIcon },
  { id: 'rose', label: 'Rose', icon: FlowerIcon },
  { id: 'amber', label: 'Amber', icon: FlameIcon },
  { id: 'mint', label: 'Mint', icon: LeafIcon },
  { id: 'sky', label: 'Sky', icon: WavesIcon },
  { id: 'ocean', label: 'Ocean', icon: WavesIcon },
  { id: 'forest', label: 'Forest', icon: LeafIcon },
  { id: 'violet', label: 'Violet', icon: SparklesIcon },
  { id: 'crimson', label: 'Crimson', icon: DropletIcon },
  { id: 'skelet', label: 'Skelet', icon: SkullIcon },
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

export type Appearance = {
  mode: ThemeMode
  palette: PaletteId
  radius: RadiusId
  scale: ScaleId
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
  reduceMotion: false,
  sidebarPalette: 'skelet',
  sidebarMode: 'dark',
}

// A palette id implies its mode, so a single id decides both halves
const DARK_PALETTES = ['ocean', 'forest', 'violet', 'crimson', 'skelet']

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
): SidebarThemeAttrs | null {
  const palette = appearance.sidebarPalette
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
  if (appearance.palette && !isCustom) root.dataset.theme = appearance.palette
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

  // Always cleared first - a stale inline value would otherwise outlive a switch back to a built-in
  for (const token of THEME_TOKENS) {
    for (const v of token.vars) root.style.removeProperty(v)
    for (const v of token.sidebarVars) root.style.removeProperty(v)
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
        reduceMotion: !!parsed.reduceMotion,
        sidebarPalette,
        sidebarMode: stored.sidebarMode === 'light' ? 'light' : 'dark',
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
}

const ThemeContext = createContext<ThemeContextValue | null>(null)

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [appearance, setAppearanceState] = useState<Appearance>(() => readStoredAppearance())
  const [custom, setCustomState] = useState<CustomTheme>(() => readCustomTheme())

  useLayoutEffect(() => {
    const media = window.matchMedia('(prefers-color-scheme: dark)')
    applyTheme(appearance, media.matches, custom)
  }, [appearance, custom])

  useEffect(() => {
    if (appearance.mode !== 'system' || appearance.palette === 'custom') return
    const media = window.matchMedia('(prefers-color-scheme: dark)')
    const onChange = () => applyTheme(appearance, media.matches, custom)
    media.addEventListener('change', onChange)
    return () => media.removeEventListener('change', onChange)
  }, [appearance, custom])

  const setAppearance = useCallback((next: Appearance) => {
    setAppearanceState(next)
    localStorage.setItem(STORAGE_KEY, JSON.stringify(next))
  }, [])

  const setCustom = useCallback((next: CustomTheme) => {
    setCustomState(next)
    localStorage.setItem(CUSTOM_KEY, JSON.stringify(next))
  }, [])

  const value = useMemo(
    () => ({ appearance, setAppearance, custom, setCustom }),
    [appearance, setAppearance, custom, setCustom],
  )
  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>
}

// eslint-disable-next-line react-refresh/only-export-components
export function useTheme() {
  const ctx = useContext(ThemeContext)
  if (!ctx) throw new Error('useTheme must be used within <ThemeProvider>')
  return ctx
}
