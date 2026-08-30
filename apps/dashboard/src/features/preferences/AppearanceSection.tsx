import { useState, type CSSProperties } from 'react'
import { CheckIcon, MoonIcon, RotateCcwIcon, SunIcon } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { DetailPageSection } from '@/components/layout/detail-page'
import { cn } from '@/lib/utils'
import {
  MODES,
  PALETTES,
  RADII,
  SCALES,
  SIDEBAR_TOKENS,
  THEME_TOKENS,
  TOKEN_GROUPS,
  defaultCustomTheme,
  sidebarColorsOf,
  type CustomTheme,
  type PaletteDef,
  type PaletteId,
  type ThemeToken,
} from '@/lib/theme'
import { seedFromPalette } from '@/lib/palette-seed'
import { isDefaultAppearance, useThemePref } from '@/lib/theme-pref'
import {
  SIDEBAR_WIDTH_DEFAULT,
  SIDEBAR_WIDTH_MAX,
  SIDEBAR_WIDTH_MIN,
  SIDEBAR_WIDTH_PRESETS,
  useSidebarWidthPref,
} from '@/lib/sidebar-pref'

const HEX = /^#[0-9a-fA-F]{6}$/

export function AppearanceSection() {
  const {
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
  } = useThemePref()
  // half-typed hex stays local so the live theme only moves on a complete value
  const [draft, setDraft] = useState<Record<string, string>>({})
  const [railDraft, setRailDraft] = useState<Record<string, string>>({})
  const isCustom = appearance.palette === 'custom'
  const railDetached = appearance.sidebarPalette !== null
  const railIsCustom = appearance.sidebarPalette === 'custom'
  const atDefault = isDefaultAppearance(appearance, custom)
  // palettes preview in the mode you'd actually get, and system resolves to the device setting
  const previewDark =
    appearance.mode === 'dark' ||
    (appearance.mode === 'system' && window.matchMedia('(prefers-color-scheme: dark)').matches)

  const setColor = (id: string, value: string) => {
    setDraft((prev) => ({ ...prev, [id]: value }))
    if (HEX.test(value)) setCustom({ ...custom, colors: { ...custom.colors, [id]: value } })
  }

  const setRailColor = (id: string, value: string) => {
    setRailDraft((prev) => ({ ...prev, [id]: value }))
    if (HEX.test(value)) {
      setCustom({ ...custom, sidebarColors: { ...custom.sidebarColors, [id]: value } })
    }
  }

  // Lifting a palette needs a canvas to resolve oklch, so a refusal says so rather than leaving a
  // button that looks broken
  const startFrom = (id: PaletteId) => {
    const seeded = seedFromPalette(id, custom.dark)
    if (!seeded) {
      toast.error("This browser won't let us read the palette's colours")
      return
    }
    setDraft({})
    setRailDraft({})
    setCustom(seeded.theme)
    // a palette that paints its rail differently only keeps that on a rail with its own palette
    if (seeded.railDiffers) setSidebarTheme('custom', custom.dark ? 'dark' : 'light')
  }

  return (
    <>
      <DetailPageSection
        title="Theme"
        description="What a new account starts with: white content, a dark sidebar, the Skelet accent and subtle corners. Everything below is yours and follows you across devices."
      >
        <div className="flex max-w-3xl flex-wrap items-center justify-between gap-3 rounded-md border px-3 py-2.5">
          <span className="text-sm text-muted-foreground">
            {atDefault ? 'You are on the default theme.' : 'Changed from the default.'}
          </span>
          <Button variant="outline" size="sm" disabled={atDefault} onClick={resetAppearance}>
            <RotateCcwIcon className="size-4" />
            Reset to default
          </Button>
        </div>
      </DetailPageSection>

      <DetailPageSection
        title="Mode"
        description={
          isCustom
            ? 'The custom palette brings its own light or dark base, set by the switch below.'
            : 'Follow the system, or pin light or dark - it applies to whichever palette you pick.'
        }
      >
        <div className="flex flex-wrap gap-2">
          {MODES.map((m) => (
            <Button
              key={m.id}
              variant="outline"
              disabled={isCustom}
              aria-pressed={appearance.mode === m.id}
              onClick={() => setMode(m.id)}
              className={cn(
                appearance.mode === m.id && !isCustom && 'border-primary bg-primary/10 text-primary',
              )}
            >
              <m.icon className="size-4" />
              {m.label}
            </Button>
          ))}
        </div>
      </DetailPageSection>

      <DetailPageSection
        title="Palette"
        description="Every palette works in both modes - each tile previews itself in the mode you've chosen."
      >
        <div className="grid gap-3 sm:grid-cols-3 lg:grid-cols-4">
          {PALETTES.map((def) => (
            <PaletteTile
              key={def.id || 'neutral'}
              def={def}
              active={appearance.palette === def.id}
              dark={def.id === 'custom' ? custom.dark : previewDark}
              vars={def.id === 'custom' ? customVars(custom.colors) : undefined}
              onSelect={() => setPalette(def.id)}
            />
          ))}
        </div>
      </DetailPageSection>

      <DetailPageSection
        title="Sidebar theme"
        description="The sidebar can wear a palette of its own - that's how the default pairs a dark rail with light content. Turn it off and the sidebar follows whatever the content uses."
      >
        <div className="divide-y overflow-hidden rounded-md border">
          <div className="flex items-center justify-between gap-3 px-3 py-2.5">
            <Label htmlFor="sidebar-own-palette" className="text-sm font-normal">
              Give the sidebar its own palette
            </Label>
            <Switch
              id="sidebar-own-palette"
              checked={railDetached}
              onCheckedChange={(on) => {
                if (!on) {
                  setSidebarPalette(null)
                  return
                }
                // Same palette, opposite base - turning this on with nothing visibly changing
                // reads as a dead switch, and a rail opposite the page is the usual intent
                setSidebarTheme(appearance.palette, previewDark ? 'light' : 'dark')
              }}
            />
          </div>
        </div>

        {railDetached && (
          <>
            <div className="flex flex-wrap gap-2">
              {(['light', 'dark'] as const).map((m) => (
                <Button
                  key={m}
                  variant="outline"
                  aria-pressed={appearance.sidebarMode === m}
                  onClick={() => setSidebarMode(m)}
                  className={cn(
                    'gap-2 capitalize',
                    appearance.sidebarMode === m && 'border-primary bg-primary/10 text-primary',
                  )}
                >
                  {m === 'dark' ? <MoonIcon className="size-4" /> : <SunIcon className="size-4" />}
                  {m}
                </Button>
              ))}
            </div>
            <div className="grid gap-3 sm:grid-cols-3 lg:grid-cols-4">
              {PALETTES.map((def) => (
                <PaletteTile
                  key={def.id || 'neutral'}
                  def={def}
                  active={appearance.sidebarPalette === def.id}
                  dark={appearance.sidebarMode === 'dark'}
                  vars={def.id === 'custom' ? customVars(sidebarColorsOf(custom, true)) : undefined}
                  onSelect={() => setSidebarPalette(def.id)}
                />
              ))}
            </div>
          </>
        )}
      </DetailPageSection>

      <DetailPageSection
        title="Corners"
        description="How rounded panels, buttons and inputs are - the whole scale follows this one choice."
      >
        <div className="flex flex-wrap gap-2">
          {RADII.map((r) => (
            <Button
              key={r.id}
              variant="outline"
              aria-pressed={appearance.radius === r.id}
              onClick={() => setRadius(r.id)}
              className={cn(
                'gap-2',
                appearance.radius === r.id && 'border-primary bg-primary/10 text-primary',
              )}
            >
              <span
                className="size-4 shrink-0 border-2 border-current"
                style={{ borderRadius: r.value }}
              />
              {r.label}
            </Button>
          ))}
        </div>
      </DetailPageSection>

      <DetailPageSection
        title="Text size"
        description="Scales the whole interface, not just the text - spacing and controls grow with it. Relative to whatever base size your browser is set to."
      >
        <div className="flex flex-wrap gap-2">
          {SCALES.map((sc) => (
            <Button
              key={sc.id}
              variant="outline"
              aria-pressed={appearance.scale === sc.id}
              onClick={() => setScale(sc.id)}
              className={cn(
                appearance.scale === sc.id && 'border-primary bg-primary/10 text-primary',
              )}
            >
              {/* the label previews its own size, so the difference is visible before you commit */}
              <span style={{ fontSize: sc.value }}>Aa</span>
              {sc.label}
            </Button>
          ))}
        </div>
      </DetailPageSection>

      <DetailPageSection
        title="Sidebar width"
        description="How wide the expanded sidebar sits. You can also drag its right edge, or double-click that edge to reset."
      >
        <SidebarWidthControls />
      </DetailPageSection>

      <DetailPageSection
        title="Motion"
        description="Cuts the sliding and fading in menus, panels and the sidebar. Independent of your system setting, which is always honoured on its own."
      >
        <div className="flex max-w-3xl items-center justify-between gap-3 rounded-md border px-3 py-2.5">
          <Label htmlFor="reduce-motion" className="text-sm font-normal">
            Reduce motion
          </Label>
          <Switch
            id="reduce-motion"
            checked={appearance.reduceMotion}
            onCheckedChange={setReduceMotion}
          />
        </div>
      </DetailPageSection>

      <DetailPageSection
        title="Custom colours"
        description="Editing any colour switches you to the Custom palette. Start from one of the built-in palettes rather than from scratch - it copies every colour across, then you change the ones you care about."
      >
        <ThemePreview custom={custom} detached={railIsCustom} />

        <div className="divide-y overflow-hidden rounded-md border">
          <div className="flex items-center justify-between gap-3 px-3 py-2.5">
            <Label htmlFor="custom-dark" className="text-sm font-normal">
              Dark base
              <span className="ml-2 text-xs text-muted-foreground">resets the colours below</span>
            </Label>
            <Switch
              id="custom-dark"
              checked={custom.dark}
              onCheckedChange={(dark) => {
                setDraft({})
                setRailDraft({})
                setCustom(defaultCustomTheme(dark))
              }}
            />
          </div>
          <div className="grid gap-2 px-3 py-2.5">
            <span className="text-sm">Start from a palette</span>
            <div className="flex flex-wrap gap-1.5">
              {PALETTES.filter((p) => p.id !== 'custom').map((p) => (
                <Button
                  key={p.id || 'neutral'}
                  variant="outline"
                  size="sm"
                  className="gap-1.5"
                  onClick={() => startFrom(p.id)}
                >
                  <p.icon className="size-3.5 text-muted-foreground" />
                  {p.label}
                </Button>
              ))}
            </div>
          </div>
        </div>

        {TOKEN_GROUPS.map((group) => {
          const tokens = THEME_TOKENS.filter((t) => t.group === group.id)
          if (!tokens.length) return null
          return (
            <div key={group.id} className="grid gap-1.5">
              <span className="text-xs font-medium text-muted-foreground">{group.label}</span>
              <div className="divide-y overflow-hidden rounded-md border">
                {tokens.map((token) => (
                  <ColorRow
                    key={token.id}
                    idPrefix="c"
                    token={token}
                    value={draft[token.id] ?? custom.colors[token.id] ?? '#000000'}
                    onChange={(v) => setColor(token.id, v)}
                  />
                ))}
              </div>
            </div>
          )
        })}

        {railIsCustom && (
          <div className="grid gap-1.5">
            <span className="text-xs font-medium text-muted-foreground">
              Sidebar
              <span className="ml-2 font-normal">anything left alone follows the colour above it</span>
            </span>
            <div className="divide-y overflow-hidden rounded-md border">
              {SIDEBAR_TOKENS.map((token) => (
                <ColorRow
                  key={token.id}
                  idPrefix="s"
                  token={token}
                  value={
                    railDraft[token.id] ??
                    custom.sidebarColors?.[token.id] ??
                    custom.colors[token.id] ??
                    '#000000'
                  }
                  onChange={(v) => setRailColor(token.id, v)}
                />
              ))}
            </div>
          </div>
        )}

        <div>
          <Button
            variant="ghost"
            size="sm"
            onClick={() => {
              setDraft({})
              setRailDraft({})
              setCustom(defaultCustomTheme(custom.dark))
            }}
          >
            Reset colours
          </Button>
        </div>
      </DetailPageSection>
    </>
  )
}

function SidebarWidthControls() {
  const { width, setWidth, resetWidth } = useSidebarWidthPref()
  return (
    <div className="flex flex-wrap items-center gap-2">
      {SIDEBAR_WIDTH_PRESETS.map((p) => (
        <Button
          key={p.label}
          variant="outline"
          aria-pressed={width === p.value}
          onClick={() => setWidth(p.value)}
          className={cn('gap-2', width === p.value && 'border-primary bg-primary/10 text-primary')}
        >
          {/* the bar previews the proportion, so the difference is visible before you commit -
              mapped across the usable range rather than scaled from zero, where four widths this
              close together all drew the same rectangle */}
          <span
            className="h-4 shrink-0 rounded-xs border-2 border-current"
            style={{
              width: `${Math.round(6 + ((p.value - SIDEBAR_WIDTH_MIN) / (SIDEBAR_WIDTH_MAX - SIDEBAR_WIDTH_MIN)) * 14)}px`,
            }}
          />
          {p.label}
        </Button>
      ))}
      <span className="ml-1 text-xs tabular-nums text-muted-foreground">{width}px</span>
      {width !== SIDEBAR_WIDTH_DEFAULT && (
        <Button variant="ghost" size="sm" onClick={resetWidth}>
          Reset
        </Button>
      )}
    </div>
  )
}

function ColorRow({
  idPrefix,
  token,
  value,
  onChange,
}: {
  idPrefix: string
  token: ThemeToken
  value: string
  onChange: (value: string) => void
}) {
  const valid = HEX.test(value)
  const id = `${idPrefix}-${token.id}`
  return (
    <div className="flex items-center gap-3 px-3 py-2">
      <Label htmlFor={id} className="flex-1 text-sm font-normal">
        {token.label}
      </Label>
      <Input
        value={value}
        onChange={(e) => onChange(e.target.value)}
        aria-label={`${token.label} hex`}
        aria-invalid={!valid}
        className="h-8 w-28 font-mono text-xs"
      />
      <input
        id={id}
        type="color"
        value={valid ? value : '#000000'}
        onChange={(e) => onChange(e.target.value)}
        aria-label={token.label}
        className="size-8 shrink-0 cursor-pointer rounded-md border bg-transparent p-0.5"
      />
    </div>
  )
}

// A shell rather than a swatch strip: a rail with its own colours is only judgeable next to the
// content it sits beside, which a row of colour chips can't show
function ThemePreview({ custom, detached }: { custom: CustomTheme; detached: boolean }) {
  const rail = sidebarColorsOf(custom, detached)
  return (
    <div
      className="flex h-28 overflow-hidden rounded-md border"
      style={{ borderColor: custom.colors.border }}
      aria-hidden
    >
      <div
        className="flex w-1/3 flex-col gap-1.5 p-2"
        style={{ background: rail.background, borderRight: `1px solid ${rail.border}` }}
      >
        <span className="h-2 w-2/3 rounded-full" style={{ background: rail.primary }} />
        <span className="h-2 rounded-full" style={{ background: rail.muted }} />
        <span className="h-2 w-4/5 rounded-full" style={{ background: rail.muted }} />
        <span
          className="h-2 w-3/5 rounded-full"
          style={{ background: rail.foreground, opacity: 0.35 }}
        />
      </div>
      <div
        className="flex flex-1 flex-col gap-1.5 p-2"
        style={{ background: custom.colors.background }}
      >
        <span className="h-2 w-1/2 rounded-full" style={{ background: custom.colors.foreground }} />
        <div
          className="flex-1 rounded-sm p-1.5"
          style={{ background: custom.colors.card, border: `1px solid ${custom.colors.border}` }}
        >
          <span
            className="block h-2 w-2/3 rounded-full"
            style={{ background: custom.colors.mutedForeground }}
          />
        </div>
        <span className="h-2 w-1/4 rounded-full" style={{ background: custom.colors.primary }} />
      </div>
    </div>
  )
}

function customVars(colors: Record<string, string>): CSSProperties {
  const out: Record<string, string> = {}
  for (const token of THEME_TOKENS) {
    const value = colors[token.id]
    if (value) for (const v of token.vars) out[v] = value
  }
  return out as CSSProperties
}

// The palettes are attribute-scoped in CSS ([data-theme='rose'], .dark[data-theme='rose']), so a
// tile carrying the same attribute and dark class renders in that palette's own variables
function PaletteTile({
  def,
  active,
  dark,
  vars,
  onSelect,
}: {
  def: PaletteDef
  active: boolean
  dark: boolean
  vars?: CSSProperties
  onSelect: () => void
}) {
  return (
    <button
      type="button"
      onClick={onSelect}
      aria-pressed={active}
      className={cn(
        'grid gap-2 rounded-md border p-2 text-left transition hover:border-primary/40',
        active && 'border-primary ring-2 ring-primary/40',
      )}
    >
      <span
        className={cn(
          'grid gap-1 overflow-hidden rounded-sm border bg-background p-2',
          dark && 'dark',
        )}
        data-theme={paletteAttr(def.id)}
        style={vars}
      >
        <span className="h-2 rounded-full bg-primary" />
        <span className="h-2 w-2/3 rounded-full bg-muted" />
        <span className="h-2 w-1/2 rounded-full border bg-card" />
      </span>
      <span className="flex items-center gap-1.5 px-0.5 text-sm">
        <def.icon className="size-3.5 shrink-0 text-muted-foreground" />
        <span className="truncate">{def.label}</span>
        {active && <CheckIcon className="ml-auto size-3.5 shrink-0 text-primary" />}
      </span>
    </button>
  )
}

// custom paints from inline vars - neutral stores as '' but previews through a real CSS block, or
// the tile would inherit whichever palette the page is currently wearing
function paletteAttr(id: PaletteId): string | undefined {
  if (id === 'custom') return undefined
  return id || 'neutral'
}
