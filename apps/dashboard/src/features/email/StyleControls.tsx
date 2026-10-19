import { useId } from 'react'
import { AlignCenterIcon, AlignLeftIcon, AlignRightIcon, XIcon } from 'lucide-react'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { cn } from '@/lib/utils'
import type { BlockStyle, EmailBlock, EmailStyles } from './blocks'

// --- small reusable controls (not exported - keep this file component-only
//     for react-refresh) ---

function ColorField({
  label,
  value,
  onChange,
}: {
  label: string
  value: string | undefined
  onChange: (v: string | undefined) => void
}) {
  const fid = useId()
  // The native picker needs a hex; fall back to black when unset/non-hex
  const swatch = value && /^#[0-9a-fA-F]{6}$/.test(value) ? value : '#000000'
  return (
    <div className="grid gap-1.5">
      <Label htmlFor={fid} className="text-xs">{label}</Label>
      <div className="flex items-center gap-1.5">
        <input
          type="color"
          value={swatch}
          onChange={(e) => onChange(e.target.value)}
          className="size-8 shrink-0 cursor-pointer rounded border bg-background p-0.5"
          aria-label={`${label} color picker`}
        />
        <Input id={fid}
          value={value ?? ''}
          onChange={(e) => onChange(e.target.value || undefined)}
          placeholder="inherit"
          className="h-8 font-mono text-xs"
        />
        {value ? (
          <button
            type="button"
            onClick={() => onChange(undefined)}
            className="rounded p-1 text-muted-foreground hover:text-foreground"
            aria-label={`Clear ${label}`}
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
  onChange,
}: {
  value: BlockStyle['align'] | undefined
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
              (value ?? 'left') === v
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

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="grid gap-2.5 border-t pt-4">
      <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">{title}</h3>
      {children}
    </div>
  )
}

// --- per-block style controls ---

const ALIGNABLE = new Set(['heading', 'text', 'button', 'image'])
const COLORABLE = new Set(['heading', 'text', 'divider'])

export function BlockStyleControls({
  block,
  onChange,
}: {
  block: EmailBlock
  onChange: (b: EmailBlock) => void
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
      {ALIGNABLE.has(block.type) && <AlignField value={s.align} onChange={(v) => patch({ align: v })} />}

      {COLORABLE.has(block.type) && (
        <ColorField label="Text color" value={s.color} onChange={(v) => patch({ color: v })} />
      )}
      {(block.type === 'heading' || block.type === 'text') && (
        <NumberField label="Font size" suffix="px" min={8} max={200} value={s.fontSize} onChange={(v) => patch({ fontSize: v })} />
      )}

      {block.type === 'button' && (
        <>
          <ColorField label="Button color" value={s.buttonColor} onChange={(v) => patch({ buttonColor: v })} />
          <ColorField label="Label color" value={s.buttonTextColor} onChange={(v) => patch({ buttonTextColor: v })} />
          <NumberField label="Corner radius" suffix="px" min={0} max={100} value={s.radius} onChange={(v) => patch({ radius: v })} />
        </>
      )}

      {block.type === 'image' && (
        <NumberField label="Width" suffix="%" min={1} max={100} value={s.width} onChange={(v) => patch({ width: v })} />
      )}

      <div className="grid grid-cols-2 gap-2">
        <NumberField label="Padding Y" suffix="px" max={200} value={s.paddingY} onChange={(v) => patch({ paddingY: v })} />
        <NumberField label="Padding X" suffix="px" max={200} value={s.paddingX} onChange={(v) => patch({ paddingX: v })} />
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
    <Section title="Theme">
      <ColorField label="Background" value={value.background} onChange={(v) => patch({ background: v })} />
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
      <ColorField label="Text color" value={value.textColor} onChange={(v) => patch({ textColor: v })} />
      <ColorField label="Link / button color" value={value.linkColor} onChange={(v) => patch({ linkColor: v })} />
    </Section>
  )
}
