import { useId } from 'react'
import { AlignCenterIcon, AlignLeftIcon, AlignRightIcon, XIcon } from 'lucide-react'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { cn } from '@/lib/utils'
import type { BlockStyle, EmailBlock, EmailStyles } from './blocks'

// --- small reusable controls (not exported - keep this file component-only
//     for react-refresh) ---

const HEX = /^#[0-9a-fA-F]{6}$/

function ColorField({
  label,
  value,
  onChange,
  fallback,
}: {
  label: string
  value: string | undefined
  onChange: (v: string | undefined) => void
  fallback?: string
}) {
  const fid = useId()
  const set = !!value
  const shown = set ? value : fallback
  return (
    <div className="grid gap-1.5">
      <Label htmlFor={fid} className="text-xs">{label}</Label>
      <div className="flex items-center gap-1.5">
        <label
          className={cn(
            'relative size-8 shrink-0 cursor-pointer overflow-hidden rounded border p-0.5',
            !set && 'border-dashed',
          )}
          title={set ? value : `Default${fallback ? ` (${fallback})` : ''} - pick a colour to change it`}
        >
          <span
            className={cn('block size-full rounded-sm', !set && 'opacity-50')}
            style={{ background: shown || 'transparent' }}
          />
          {!set ? <span className="absolute inset-0 m-auto h-px w-[140%] -translate-x-[15%] -rotate-45 bg-muted-foreground/70" /> : null}
          <input
            type="color"
            // The native picker needs a hex - it opens on the default, so a tweak starts from what's shown
            value={shown && HEX.test(shown) ? shown : '#ffffff'}
            onChange={(e) => onChange(e.target.value)}
            className="absolute inset-0 size-full cursor-pointer opacity-0"
            aria-label={`${label} color picker`}
          />
        </label>
        <Input id={fid}
          value={value ?? ''}
          onChange={(e) => onChange(e.target.value || undefined)}
          placeholder={fallback ? `Default (${fallback})` : 'None'}
          className="h-8 font-mono text-xs"
        />
        {set ? (
          <button
            type="button"
            onClick={() => onChange(undefined)}
            className="rounded p-1 text-muted-foreground hover:text-foreground"
            aria-label={`Reset ${label} to default`}
            title="Reset to default"
          >
            <XIcon className="size-3.5" />
          </button>
        ) : null}
      </div>
    </div>
  )
}

function NumberField({
  label,
  value,
  onChange,
  min = 0,
  max = 999,
  placeholder,
  suffix,
}: {
  label: string
  value: number | undefined
  onChange: (v: number | undefined) => void
  min?: number
  max?: number
  placeholder?: string
  suffix?: string
}) {
  const fid = useId()
  return (
    <div className="grid gap-1.5">
      <Label htmlFor={fid} className="text-xs">
        {label}
        {suffix ? <span className="text-muted-foreground"> ({suffix})</span> : null}
      </Label>
      <Input id={fid}
        type="number"
        min={min}
        max={max}
        value={value ?? ''}
        placeholder={placeholder}
        onChange={(e) => {
          const v = e.target.value
          onChange(v === '' ? undefined : Math.max(min, Math.min(max, Number(v))))
        }}
        className="h-8 text-xs"
      />
    </div>
  )
}

function AlignField({
  value,
  fallback = 'left',
  onChange,
}: {
  value: BlockStyle['align'] | undefined
  fallback?: 'left' | 'center' | 'right'
  onChange: (v: BlockStyle['align']) => void
}) {
  const opts: Array<{ v: 'left' | 'center' | 'right'; icon: typeof AlignLeftIcon }> = [
    { v: 'left', icon: AlignLeftIcon },
    { v: 'center', icon: AlignCenterIcon },
    { v: 'right', icon: AlignRightIcon },
  ]
  return (
    <div className="grid gap-1.5">
      <Label className="text-xs">Alignment</Label>
      <div className="flex gap-1">
        {opts.map(({ v, icon: Icon }) => (
          <button
            key={v}
            type="button"
            onClick={() => onChange(v)}
            className={cn(
              'flex h-8 flex-1 items-center justify-center rounded border transition-colors',
              (value ?? fallback) === v
                ? 'border-primary bg-primary/10 text-primary'
                : 'bg-background text-muted-foreground hover:bg-accent',
            )}
            aria-label={`Align ${v}`}
          >
            <Icon className="size-4" />
          </button>
        ))}
      </div>
    </div>
  )
}

function Section({ title, first, children }: { title: string; first?: boolean; children: React.ReactNode }) {
  return (
    <div className={cn('grid gap-2.5', !first && 'border-t pt-4')}>
      <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">{title}</h3>
      {children}
    </div>
  )
}

// --- per-block style controls ---

const ALIGNABLE = new Set(['heading', 'text', 'button', 'image', 'coupon', 'image_text', 'menu', 'store_footer'])
const COLORABLE = new Set(['heading', 'text', 'divider', 'coupon', 'image_text', 'menu', 'store_footer'])
// These sit centred until told otherwise, as the server draws them
const CENTERED = new Set(['coupon', 'menu', 'store_footer'])
// The blocks that end on a button take its colours too
const HAS_BUTTON = new Set(['button', 'coupon', 'image_text'])

export function BlockStyleControls({
  block,
  onChange,
  theme = {},
}: {
  block: EmailBlock
  onChange: (b: EmailBlock) => void
  theme?: EmailStyles
}) {
  const s = block.style ?? {}
  const patch = (p: Partial<BlockStyle>) => {
    const next = { ...s, ...p }
    // drop undefined keys so empty styles serialize cleanly
    for (const k of Object.keys(next) as Array<keyof BlockStyle>) {
      if (next[k] === undefined || next[k] === '') delete next[k]
    }
    onChange({ ...block, style: Object.keys(next).length ? next : undefined })
  }

  return (
    <Section title="Style">
      {ALIGNABLE.has(block.type) && (
        <AlignField value={s.align} fallback={CENTERED.has(block.type) ? 'center' : 'left'} onChange={(v) => patch({ align: v })} />
      )}

      {COLORABLE.has(block.type) && (
        <ColorField
          label={block.type === 'divider' ? 'Line color' : block.type === 'menu' ? 'Link color' : 'Text color'}
          value={s.color}
          fallback={
            block.type === 'divider'
              ? '#e5e5e5'
              : block.type === 'store_footer'
                ? '#888888'
                : block.type === 'menu'
                  ? theme.linkColor || '#222222'
                  : theme.textColor || (block.type === 'heading' || block.type === 'coupon' ? '#222222' : '#333333')
          }
          onChange={(v) => patch({ color: v })}
        />
      )}
      {(block.type === 'heading' || block.type === 'text') && (
        <NumberField
          label="Font size"
          suffix="px"
          min={8}
          max={200}
          placeholder={block.type === 'heading' ? '20' : 'Default'}
          value={s.fontSize}
          onChange={(v) => patch({ fontSize: v })}
        />
      )}

      {HAS_BUTTON.has(block.type) && (
        <>
          <ColorField label="Button color" value={s.buttonColor} fallback={theme.linkColor || '#222222'} onChange={(v) => patch({ buttonColor: v })} />
          <ColorField label="Label color" value={s.buttonTextColor} fallback="#ffffff" onChange={(v) => patch({ buttonTextColor: v })} />
          <NumberField label="Corner radius" suffix="px" min={0} max={100} placeholder="4" value={s.radius} onChange={(v) => patch({ radius: v })} />
        </>
      )}

      {block.type === 'image' && (
        <NumberField label="Width" suffix="%" min={1} max={100} placeholder="Natural size" value={s.width} onChange={(v) => patch({ width: v })} />
      )}

      <div className="grid grid-cols-2 gap-2">
        <NumberField label="Padding Y" suffix="px" max={200} placeholder="0" value={s.paddingY} onChange={(v) => patch({ paddingY: v })} />
        <NumberField label="Padding X" suffix="px" max={200} placeholder="0" value={s.paddingX} onChange={(v) => patch({ paddingX: v })} />
      </div>
      <ColorField label="Background" value={s.background} onChange={(v) => patch({ background: v })} />

      <label className="flex items-center justify-between gap-2 pt-1 text-xs" htmlFor="block-hide-mobile">
        <span className="text-muted-foreground">Hide on mobile</span>
        <Switch
          id="block-hide-mobile"
          checked={!!s.hideMobile}
          onCheckedChange={(on) => patch({ hideMobile: on ? true : undefined })}
        />
      </label>
    </Section>
  )
}

// --- global email theme controls ---

export function GlobalStyleControls({
  value,
  onChange,
}: {
  value: EmailStyles
  onChange: (s: EmailStyles) => void
}) {
  const fid = useId()
  const patch = (p: Partial<EmailStyles>) => {
    const next = { ...value, ...p }
    for (const k of Object.keys(next) as Array<keyof EmailStyles>) {
      if (next[k] === undefined || next[k] === '') delete next[k]
    }
    onChange(next)
  }
  return (
    <Section title="Theme" first>
      <p className="text-xs text-muted-foreground">Applies to the whole email. A block's own style wins over it.</p>
      <ColorField label="Background" value={value.background} fallback="#f6f6f6" onChange={(v) => patch({ background: v })} />
      <NumberField label="Content width" suffix="px" min={200} max={1000} placeholder="600" value={value.contentWidth} onChange={(v) => patch({ contentWidth: v })} />
      <div className="grid gap-1.5">
        <Label htmlFor={`${fid}-font-family`} className="text-xs">Font family</Label>
        <Input id={`${fid}-font-family`}
          value={value.fontFamily ?? ''}
          onChange={(e) => patch({ fontFamily: e.target.value || undefined })}
          placeholder="Helvetica, Arial, sans-serif"
          className="h-8 text-xs"
        />
      </div>
      <ColorField label="Text color" value={value.textColor} fallback="#333333" onChange={(v) => patch({ textColor: v })} />
      <ColorField label="Link / button color" value={value.linkColor} fallback="#222222" onChange={(v) => patch({ linkColor: v })} />
    </Section>
  )
}
