import { useState, type CSSProperties, type ReactNode } from 'react'
import { CheckIcon, MoonIcon, RotateCcwIcon, SunIcon } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { DetailPageSection } from '@/components/layout/detail-page'
import { cn } from '@/lib/utils'
import {
  DENSITIES,
  FRAMES,
  MODES,
  PALETTES,
  RADII,
  SCALES,
  SIDEBAR_TOKENS,
  THEME_PRESETS,
  THEME_TOKENS,
  TOKEN_GROUPS,
  TYPEFACES,
  defaultCustomTheme,
  presetLook,
  presetMatches,
  sidebarColorsOf,
  type CustomTheme,
  type DensityId,
  type FrameId,
  type PaletteDef,
  type PaletteId,
  type RadiusId,
  type ThemePreset,
  type ThemeToken,
  type TypefaceId,
} from '@/lib/theme'
import { DEFAULT_TUNE } from '@/lib/palette-recipe'
import { seedFromPalette } from '@/lib/palette-seed'
import {
  ACCENT_SWATCHES,
  DARK_BASES,
  RAIL_TONES,
  SURFACE_TINTS,
  SURFACE_TONES,
  VIVIDNESS,
  tunedVars,
} from '@/lib/palette-recipe'
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
  } = useThemePref()
  // half-typed hex stays local so the live theme only moves on a complete value
  const [draft, setDraft] = useState<Record<string, string>>({})
  const [railDraft, setRailDraft] = useState<Record<string, string>>({})
  const isCustom = appearance.palette === 'custom'
  const isTuned = appearance.palette === 'tuned'
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
    const seeded = seedFromPalette(id, custom.dark, id === 'tuned' ? tunedVars(appearance, custom.dark) : undefined)
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
        title="Themes"
        description="A whole look in one click - colours, sidebar, corners, typeface and spacing together. Change anything below afterwards and the theme simply stops being highlighted."
        trailing={
          <Button variant="outline" size="sm" disabled={atDefault} onClick={resetAppearance}>
            <RotateCcwIcon className="size-4" />
            Reset to default
          </Button>
        }
      >
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
          {THEME_PRESETS.map((preset) => (
            <PresetTile
              key={preset.id}
              preset={preset}
              active={presetMatches(preset, appearance)}
              onSelect={() => applyPreset(preset)}
            />
          ))}
        </div>
      </DetailPageSection>

      <DetailPageSection
        title="Colour"
        description={
          isCustom
            ? 'The custom palette brings its own light or dark base, set under Custom colours.'
            : 'Pick light or dark and a base. Skelet, Neutral and High contrast are fixed; turn any dial below and you get a Tuned palette of your own.'
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
        <div className="grid gap-3 sm:grid-cols-3 lg:grid-cols-5">
          {PALETTES.map((def) => (
            <PaletteTile
              key={def.id || 'neutral'}
              def={def}
              active={appearance.palette === def.id}
              dark={def.id === 'custom' ? custom.dark : previewDark}
              vars={
                def.id === 'custom'
                  ? customVars(custom.colors)
                  : def.id === 'tuned'
                    ? (tunedVars(appearance, previewDark) as CSSProperties)
                    : undefined
              }
              onSelect={() => (def.id === 'tuned' ? setTune({}) : setPalette(def.id))}
            />
          ))}
        </div>

        <div className="grid gap-4 rounded-md border p-3">
          <div className="grid gap-2">
            <span className="text-sm">Accent</span>
            <div className="flex flex-wrap items-center gap-2">
              {ACCENT_SWATCHES.map((sw) => {
                const on = isTuned && appearance.vividness !== 'mono' && appearance.accentHue === sw.hue
                return (
                  <button
                    key={sw.hue}
                    type="button"
                    title={sw.label}
                    aria-label={sw.label}
                    aria-pressed={on}
                    onClick={() => setTune({ accentHue: sw.hue, vividness: appearance.vividness === 'mono' ? 'balanced' : appearance.vividness })}
                    className={cn('size-7 rounded-full border border-black/10 transition', on && 'ring-2 ring-ring ring-offset-2 ring-offset-background')}
                    style={{ background: `oklch(0.64 0.17 ${sw.hue})` }}
                  />
                )
              })}
            </div>
            <input
              type="range"
              min={0}
              max={359}
              value={appearance.accentHue}
              aria-label="Accent hue"
              onChange={(e) => setTune({ accentHue: Number(e.target.value) })}
              className="h-2 w-full max-w-md cursor-pointer appearance-none rounded-full accent-primary"
              style={{ background: HUE_TRACK }}
            />
          </div>
          <DialRow label="Vividness" options={VIVIDNESS} value={isTuned ? appearance.vividness : null} onPick={(vividness) => setTune({ vividness })} />
          <DialRow label="Surfaces" options={SURFACE_TONES} value={isTuned ? appearance.surfaceTone : null} onPick={(surfaceTone) => setTune({ surfaceTone })} />
          {appearance.surfaceTone !== 'neutral' && (
            <DialRow label="Tint strength" options={SURFACE_TINTS} value={isTuned ? appearance.surfaceTint : null} onPick={(surfaceTint) => setTune({ surfaceTint })} />
          )}
          <DialRow label="Dark base" hint="used in dark mode" options={DARK_BASES} value={isTuned ? appearance.darkBase : null} onPick={(darkBase) => setTune({ darkBase })} />
        </div>
      </DetailPageSection>

      <DetailPageSection
        title="Sidebar"
        description={
          isTuned
            ? 'How the rail is coloured against your tuned page, then how it sits on screen.'
            : "The sidebar can wear a palette of its own - that's how the default pairs a dark rail with light content. Turn it off and it follows the content."
        }
      >
        {isTuned ? (
          <DialRow label="Rail colour" options={RAIL_TONES} value={railDetached ? null : appearance.railTone} onPick={(railTone) => setTune({ railTone })} />
        ) : (
        <>
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
            <div className="grid gap-3 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-6">
              {PALETTES.filter((def) => def.id !== 'tuned').map((def) => (
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
        </>
        )}

        <ChoiceRow label="Style">
          {FRAMES.map((f) => (
            <ChoiceButton key={f.id} active={appearance.frame === f.id} onClick={() => setFrame(f.id)}>
              <FrameGlyph id={f.id} />
              {f.label}
            </ChoiceButton>
          ))}
        </ChoiceRow>
        <ChoiceRow label="Side">
          {(['left', 'right'] as const).map((side) => (
            <ChoiceButton key={side} active={appearance.sidebarSide === side} onClick={() => setSidebarSide(side)}>
              <span className={cn('flex h-3.5 w-4 rounded-[2px] border border-current', side === 'right' && 'flex-row-reverse')}>
                <span className="w-1.5 bg-current" />
              </span>
              <span className="capitalize">{side}</span>
            </ChoiceButton>
          ))}
        </ChoiceRow>
        <div className="grid gap-2">
          <span className="text-sm">Width</span>
          <SidebarWidthControls />
          <span className="text-xs text-muted-foreground">
            You can also drag the sidebar's inner edge, or double-click it to reset.
          </span>
        </div>
      </DetailPageSection>

      <DetailPageSection
        title="Type and layout"
        description="How text is set and how tightly the interface packs. Text size scales everything; spacing only moves the gaps, so more fits on screen without smaller text."
      >
        <ChoiceRow label="Typeface">
          {TYPEFACES.map((t) => (
            <ChoiceButton key={t.id} active={appearance.typeface === t.id} onClick={() => setTypeface(t.id)}>
              {/* the label is set in its own face, so the difference shows before you pick */}
              <span style={{ fontFamily: t.sample }}>{t.label}</span>
            </ChoiceButton>
          ))}
        </ChoiceRow>
        <ChoiceRow label="Text size">
          {SCALES.map((sc) => (
            <ChoiceButton key={sc.id} active={appearance.scale === sc.id} onClick={() => setScale(sc.id)}>
              <span style={{ fontSize: sc.value }}>Aa</span>
              {sc.label}
            </ChoiceButton>
          ))}
        </ChoiceRow>
        <ChoiceRow label="Spacing">
          {DENSITIES.map((d) => (
            <ChoiceButton key={d.id} active={appearance.density === d.id} onClick={() => setDensity(d.id)}>
              <DensityGlyph id={d.id} />
              {d.label}
            </ChoiceButton>
          ))}
        </ChoiceRow>
        <ChoiceRow label="Corners">
          {RADII.map((r) => (
            <ChoiceButton key={r.id} active={appearance.radius === r.id} onClick={() => setRadius(r.id)}>
              <span className="size-4 shrink-0 border-2 border-current" style={{ borderRadius: r.value }} />
              {r.label}
            </ChoiceButton>
          ))}
        </ChoiceRow>
        <div className="flex max-w-3xl items-center justify-between gap-3 rounded-md border px-3 py-2.5">
          <Label htmlFor="reduce-motion" className="grid gap-0.5 text-sm font-normal">
            Reduce motion
            <span className="text-xs text-muted-foreground">
              Cuts the sliding and fading in menus, panels and the sidebar. Your system setting is always honoured on its own.
            </span>
          </Label>
          <Switch id="reduce-motion" checked={appearance.reduceMotion} onCheckedChange={setReduceMotion} />
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

// Custom paints from inline variables - both sets, so the preview's rail reads the same colours the
// real sidebar would
function customVars(colors: Record<string, string>): CSSProperties {
  const out: Record<string, string> = {}
  for (const token of THEME_TOKENS) {
    const value = colors[token.id]
    if (!value) continue
    for (const v of token.vars) out[v] = value
    for (const v of token.sidebarVars) out[v] = value
  }
  return out as CSSProperties
}

// Neutral stores as '' but previews through its real CSS block, or a tile would inherit whichever
// palette the page is wearing; custom has no block and paints from inline variables instead
function paletteAttr(id: PaletteId): string | undefined {
  if (id === 'custom' || id === 'tuned') return undefined
  return id || 'neutral'
}

type PreviewLook = {
  palette: PaletteId
  dark: boolean
  // undefined - the rail follows the content and reads the content's own sidebar colours
  rail?: { palette: PaletteId; dark: boolean }
  radius?: RadiusId
  typeface?: TypefaceId
  density?: DensityId
  frame?: FrameId
  side?: 'left' | 'right'
  vars?: CSSProperties
  railVars?: CSSProperties
}

// A tiny app - rail, title, a card and a button - drawn with the theme's own variables. The palettes
// are attribute-scoped in CSS, so an element carrying the same data-theme and .dark renders in that
// palette; radius, spacing and font are set on the same element, so corners, gaps and type follow
function ShellPreview({ look }: { look: PreviewLook }) {
  const face = TYPEFACES.find((t) => t.id === (look.typeface ?? 'geist'))
  const radius = RADII.find((r) => r.id === (look.radius ?? 'subtle'))
  const spacing = DENSITIES.find((d) => d.id === (look.density ?? 'default'))
  const scopeStyle = {
    ...look.vars,
    '--radius': radius?.value,
    '--spacing': spacing?.value,
    fontFamily: face?.id === 'editorial' ? undefined : face?.sample,
  } as CSSProperties
  const titleFace = look.typeface === 'editorial' ? TYPEFACES.find((t) => t.id === 'serif')?.sample : undefined
  const frame = look.frame ?? 'docked'
  const right = look.side === 'right'
  return (
    <span
      aria-hidden
      className={cn('relative flex h-[96px] overflow-hidden rounded-md border bg-background text-foreground', right && 'flex-row-reverse', look.dark && 'dark')}
      data-theme={paletteAttr(look.palette)}
      style={scopeStyle}
    >
      {/* a rail with its own palette is its own scope, the same way the real sidebar is; inset's
          gutter is painted from inside that scope too */}
      <span
        className={cn('flex w-1/4', frame === 'floating' && 'p-1', frame === 'inset' && 'absolute inset-0 w-full', look.rail?.dark && 'dark')}
        data-theme={look.rail ? paletteAttr(look.rail.palette) : undefined}
        style={look.railVars}
      >
        <span className={cn('flex', frame === 'inset' ? 'w-1/4 bg-sidebar' : 'w-full', right && 'ml-auto', frame === 'floating' && 'overflow-hidden rounded-md ring-1 ring-sidebar-border')}>
          <RailBody bare={frame !== 'docked'} right={right} />
        </span>
      </span>
      {frame === 'inset' && <span className="w-1/4 shrink-0" />}
      <span
        className={cn(
          'relative flex flex-1 flex-col gap-1.5 bg-background p-2',
          frame === 'inset' && cn('my-1 rounded-md shadow-sm', right ? 'ml-1' : 'mr-1'),
        )}
      >
        <span className="text-[11px] leading-none font-semibold" style={{ fontFamily: titleFace }}>
          Orders
        </span>
        <span className="grid flex-1 content-start gap-1 rounded-md border bg-card p-1.5">
          <span className="h-1.5 w-3/4 rounded-full bg-muted-foreground/45" />
          <span className="h-1.5 w-1/2 rounded-full bg-muted-foreground/25" />
        </span>
        <span className="flex items-center gap-1">
          <span className="h-3 w-8 rounded-md bg-primary" />
          <span className="h-3 w-5 rounded-md bg-muted" />
        </span>
      </span>
    </span>
  )
}

function RailBody({ bare = false, right = false }: { bare?: boolean; right?: boolean }) {
  return (
    <span className={cn('flex w-full flex-col gap-1 bg-sidebar p-1.5', !bare && (right ? 'border-l' : 'border-r'), 'border-sidebar-border')}>
      <span className="size-2 rounded-full bg-sidebar-primary" />
      <span className="mt-1 h-2 rounded-sm bg-sidebar-accent" />
      <span className="h-1.5 w-4/5 rounded-full bg-sidebar-foreground/30" />
      <span className="h-1.5 w-3/5 rounded-full bg-sidebar-foreground/30" />
      <span className="h-1.5 w-2/3 rounded-full bg-sidebar-foreground/30" />
    </span>
  )
}

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
      <ShellPreview look={{ palette: def.id, dark, vars }} />
      <span className="flex items-center gap-1.5 px-0.5 text-sm">
        <def.icon className="size-3.5 shrink-0 text-muted-foreground" />
        <span className="truncate">{def.label}</span>
        {active && <CheckIcon className="ml-auto size-3.5 shrink-0 text-primary" />}
      </span>
    </button>
  )
}

function PresetTile({ preset, active, onSelect }: { preset: ThemePreset; active: boolean; onSelect: () => void }) {
  const look = presetLook(preset)
  return (
    <button
      type="button"
      onClick={onSelect}
      aria-pressed={active}
      className={cn(
        'grid content-start gap-2 rounded-md border p-2 text-left transition hover:border-primary/40',
        active && 'border-primary ring-2 ring-primary/40',
      )}
    >
      <ShellPreview
        look={{
          palette: look.palette,
          dark: look.mode === 'dark',
          rail: look.sidebarPalette === null ? undefined : { palette: look.sidebarPalette, dark: look.sidebarMode === 'dark' },
          radius: look.radius,
          typeface: look.typeface,
          density: look.density,
          frame: look.frame,
          side: look.sidebarSide,
          vars: look.palette === 'tuned' ? (tunedVars({ ...DEFAULT_TUNE, ...look }, look.mode === 'dark') as CSSProperties) : undefined,
        }}
      />
      <span className="grid gap-0.5 px-0.5">
        <span className="flex items-center gap-1.5 text-sm font-medium">
          {preset.label}
          {active && <CheckIcon className="ml-auto size-3.5 shrink-0 text-primary" />}
        </span>
        <span className="text-xs text-muted-foreground">{preset.description}</span>
      </span>
    </button>
  )
}

function ChoiceRow({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="grid gap-2">
      <span className="text-sm">{label}</span>
      <div className="flex flex-wrap gap-2">{children}</div>
    </div>
  )
}

function ChoiceButton({ active, onClick, children }: { active: boolean; onClick: () => void; children: ReactNode }) {
  return (
    <Button
      variant="outline"
      aria-pressed={active}
      onClick={onClick}
      className={cn('gap-2', active && 'border-primary bg-primary/10 text-primary')}
    >
      {children}
    </Button>
  )
}

// Three bars spaced the way the density packs them
function DensityGlyph({ id }: { id: DensityId }) {
  const gap = id === 'compact' ? 1 : id === 'default' ? 2 : 3
  return (
    <span className="flex w-4 flex-col justify-center" style={{ gap }}>
      <span className="h-0.5 rounded-full bg-current" />
      <span className="h-0.5 rounded-full bg-current" />
      <span className="h-0.5 rounded-full bg-current" />
    </span>
  )
}

// A rail and a page, drawn the way each style frames them
function FrameGlyph({ id }: { id: FrameId }) {
  return (
    <span className={cn('flex h-3.5 w-4 gap-px rounded-[2px] border border-current', id !== 'docked' && 'p-px')}>
      <span className={cn('w-1.5 bg-current', id === 'floating' && 'rounded-[1px]')} />
      {id === 'inset' && <span className="flex-1 rounded-[1px] border border-current" />}
    </span>
  )
}

const HUE_TRACK = `linear-gradient(to right, ${Array.from({ length: 13 }, (_, i) => `oklch(0.68 0.16 ${i * 30})`).join(', ')})`

function DialRow<T extends string>({
  label,
  hint,
  options,
  value,
  onPick,
}: {
  label: string
  hint?: string
  options: { id: T; label: string }[]
  // null while another base is active - the dials show, but none reads as chosen
  value: T | null
  onPick: (id: T) => void
}) {
  return (
    <div className="grid gap-2">
      <span className="text-sm">
        {label}
        {hint && <span className="ml-2 text-xs text-muted-foreground">{hint}</span>}
      </span>
      <div className="flex flex-wrap gap-2">
        {options.map((o) => (
          <ChoiceButton key={o.id} active={value === o.id} onClick={() => onPick(o.id)}>
            {o.label}
          </ChoiceButton>
        ))}
      </div>
    </div>
  )
}
