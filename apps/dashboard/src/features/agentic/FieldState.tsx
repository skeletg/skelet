import type { KeyboardEvent } from 'react'
import type { ConfigFieldData } from '@/features/settings/ConfigField'
import { cn } from '@/lib/utils'

export type Field = {
  data: ConfigFieldData
  value: string
  dirty: boolean
  set: (value: string) => void
  reset: () => void
  resetting: boolean
  inherited: boolean
}

export function FieldState({
  field,
  here,
  from,
  locked,
  count,
  max,
  note,
}: {
  field: Field
  here: string
  from: string
  locked?: boolean
  count?: number
  max?: number
  note?: string
}) {
  return (
    <div className="mt-1.5 flex min-h-5 flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
      {note ? <span>{note}</span> : null}
      {field.dirty ? (
        <span className="font-medium text-warning">Unsaved</span>
      ) : field.inherited ? (
        <span>From {from}</span>
      ) : (
        <span className="flex items-center gap-2">
          Set {here}
          {locked ? null : (
            <button
              type="button"
              onClick={field.reset}
              disabled={field.resetting}
              aria-label={`Reset ${field.data.label}`}
              className="underline underline-offset-2 hover:text-foreground"
            >
              Reset
            </button>
          )}
        </span>
      )}
      {count !== undefined && max ? (
        <span className={cn('ml-auto tabular-nums', count > max * 0.9 && 'text-warning')}>
          {count.toLocaleString()} / {max.toLocaleString()}
        </span>
      ) : null}
    </div>
  )
}

type Option = { value: string; label: string }

export function SegmentedRadio({
  labelledBy,
  options,
  value,
  onChange,
  disabled,
  columns = options.length,
  className,
}: {
  labelledBy: string
  options: Option[]
  value: string
  onChange: (value: string) => void
  disabled?: boolean
  columns?: number
  className?: string
}) {
  const anyChecked = options.some((o) => o.value === value)
  const onKey = (e: KeyboardEvent<HTMLButtonElement>, i: number) => {
    const step = e.key === 'ArrowRight' || e.key === 'ArrowDown' ? 1 : e.key === 'ArrowLeft' || e.key === 'ArrowUp' ? -1 : 0
    if (!step) return
    e.preventDefault()
    const next = (i + step + options.length) % options.length
    onChange(options[next].value)
    e.currentTarget.parentElement?.querySelectorAll<HTMLButtonElement>('[role="radio"]')[next]?.focus()
  }
  return (
    <div
      role="radiogroup"
      aria-labelledby={labelledBy}
      className={cn('grid gap-1 rounded-lg border bg-background/70 p-1', className)}
      style={{ gridTemplateColumns: `repeat(${Math.max(1, columns)}, minmax(0, 1fr))` }}
    >
      {options.map((o, i) => {
        const checked = o.value === value
        return (
          <button
            key={o.value}
            type="button"
            role="radio"
            aria-checked={checked}
            tabIndex={checked || (!anyChecked && i === 0) ? 0 : -1}
            disabled={disabled}
            onClick={() => onChange(o.value)}
            onKeyDown={(e) => onKey(e, i)}
            className={cn(
              'rounded-md px-3 py-1.5 text-sm transition-colors focus-visible:outline-none focus-visible:ring-3 focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-60',
              checked ? 'bg-foreground text-background' : 'text-muted-foreground hover:bg-muted hover:text-foreground',
            )}
          >
            {o.label}
          </button>
        )
      })}
    </div>
  )
}
