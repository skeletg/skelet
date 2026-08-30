import { SIDEBAR_TOKENS, THEME_TOKENS, defaultCustomTheme, type CustomTheme, type PaletteId } from '@/lib/theme'

// Lifts a built-in palette into the custom theme's hex map, so "start from Ocean and change two
// things" replaces "type ten hex codes and hope they cohere"
//
// The palettes live in index.css as oklch, some with alpha (--sidebar-border is white at 10%), and
// the pickers only speak #rrggbb. Rather than parse oklch by hand we let the browser do it: paint
// the colour onto a 1x1 canvas over the palette's own background and read the pixel back, which
// resolves the colour space AND flattens alpha against the surface it actually sits on

function sampleCanvas(): CanvasRenderingContext2D | null {
  try {
    const canvas = document.createElement('canvas')
    canvas.width = 1
    canvas.height = 1
    return canvas.getContext('2d', { willReadFrequently: true })
  } catch {
    return null // jsdom and any engine without a 2d context - callers fall back to the defaults
  }
}

function toHex(ctx: CanvasRenderingContext2D, color: string, backdrop: string): string | null {
  if (!color.trim()) return null
  ctx.clearRect(0, 0, 1, 1)
  // An unparseable value leaves fillStyle untouched, so a sentinel tells us the assignment took
  const SENTINEL = '#010203'
  ctx.fillStyle = SENTINEL
  ctx.fillStyle = backdrop
  if (ctx.fillStyle === SENTINEL && backdrop !== SENTINEL) return null
  ctx.fillRect(0, 0, 1, 1)
  ctx.fillStyle = SENTINEL
  ctx.fillStyle = color
  if (ctx.fillStyle === SENTINEL && color !== SENTINEL) return null
  ctx.fillRect(0, 0, 1, 1)
  try {
    const [r, g, b] = ctx.getImageData(0, 0, 1, 1).data
    return `#${[r, g, b].map((c) => c.toString(16).padStart(2, '0')).join('')}`
  } catch {
    return null // a tainted canvas can't happen here, but a locked-down engine can still refuse
  }
}

// Neutral stores as '' but paints through a real [data-theme='neutral'] block, and custom has no
// block at all - there is nothing to lift from it
function paletteAttr(id: PaletteId): string | null {
  if (id === 'custom') return null
  return id || 'neutral'
}

// Mounted rather than detached: custom properties only resolve through getComputedStyle once the
// element is in the document, and off-screen keeps it from flashing
function withPaletteProbe<T>(attr: string, dark: boolean, fn: (probe: HTMLElement) => T): T {
  const probe = document.createElement('div')
  probe.dataset.theme = attr
  if (dark) probe.classList.add('dark')
  probe.style.cssText = 'position:fixed;left:-9999px;top:0;width:0;height:0;pointer-events:none'
  document.body.appendChild(probe)
  try {
    return fn(probe)
  } finally {
    probe.remove()
  }
}

// `railDiffers` reports that this palette paints its sidebar unlike its page, which is the cue to
// hand the rail its own palette rather than leaving it mirroring the content
export type SeededPalette = { theme: CustomTheme; railDiffers: boolean }

// Reads one palette in one mode and returns a fully-populated custom theme. Falls back per token,
// so a variable the palette doesn't define keeps the built-in default rather than blanking
export function seedFromPalette(id: PaletteId, dark: boolean): SeededPalette | null {
  const attr = paletteAttr(id)
  if (!attr) return null
  const ctx = sampleCanvas()
  if (!ctx) return null

  return withPaletteProbe(attr, dark, (probe) => {
    const style = getComputedStyle(probe)
    const read = (v: string) => style.getPropertyValue(v)
    const base = defaultCustomTheme(dark)
    const pageBg = toHex(ctx, read('--background'), dark ? '#000000' : '#ffffff') ?? base.colors.background
    const railBg = toHex(ctx, read('--sidebar'), pageBg) ?? pageBg

    const colors: Record<string, string> = { ...base.colors }
    for (const token of THEME_TOKENS) {
      // the first var is the role's canonical one - the rest are aliases pointing at the same value
      const hex = toHex(ctx, read(token.vars[0]), pageBg)
      if (hex) colors[token.id] = hex
    }

    const sidebarColors: Record<string, string> = {}
    for (const token of SIDEBAR_TOKENS) {
      const hex = toHex(ctx, read(token.sidebarVars[0]), railBg)
      if (hex) sidebarColors[token.id] = hex
    }

    // Only worth detaching if the palette actually paints its rail differently
    const railDiffers = SIDEBAR_TOKENS.some(
      (t) => sidebarColors[t.id] && sidebarColors[t.id] !== colors[t.id],
    )
    return { theme: { dark, colors, sidebarColors }, railDiffers }
  })
}
