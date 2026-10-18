import { useId, type ReactNode } from 'react'
import { Badge } from '@/components/ui/badge'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { STATUS_LABELS, STATUS_VARIANTS, type OptionStatus } from './labels'

const NONE = '__none__'

// A labelled control with an optional hint underneath - the one row shape every checkout editor uses
export function Row({ label, hint, htmlFor, children }: { label: ReactNode; hint?: ReactNode; htmlFor?: string; children: ReactNode }) {
  return (
    <div className="grid gap-1.5">
      <Label htmlFor={htmlFor}>{label}</Label>
      {children}
      {hint ? <p className="text-xs text-muted-foreground">{hint}</p> : null}
    </div>
  )
}

// A plain string select over the ui Select - `noneLabel` stands for '', which Radix can't hold as a value
export function Choice({
  label,
  hint,
  value,
  onChange,
  options,
  noneLabel,
  placeholder,
  disabled,
}: {
  label: ReactNode
  hint?: ReactNode
  value: string
  onChange: (value: string) => void
  options: readonly { value: string; label: string }[]
  noneLabel?: string
  placeholder?: string
  disabled?: boolean
}) {
  const id = useId()
  const current = value === '' && noneLabel !== undefined ? NONE : value
  return (
    <Row label={label} hint={hint} htmlFor={id}>
      <Select value={current} disabled={disabled} onValueChange={(v) => onChange(v === NONE ? '' : v)}>
        <SelectTrigger id={id} className="w-full">
          <SelectValue placeholder={placeholder} />
        </SelectTrigger>
        <SelectContent>
          {noneLabel !== undefined && <SelectItem value={NONE}>{noneLabel}</SelectItem>}
          {options.map((o) => (
            <SelectItem key={o.value} value={o.value}>
              {o.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </Row>
  )
}

export function StatusBadge({ status }: { status: OptionStatus }) {
  return <Badge variant={STATUS_VARIANTS[status]}>{STATUS_LABELS[status]}</Badge>
}

// A titled block inside a tab - flat and bordered, per the dashboard's no-cards rule
export function Block({ title, description, actions, children }: { title: ReactNode; description?: ReactNode; actions?: ReactNode; children: ReactNode }) {
  return (
    <div className="grid gap-3">
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div className="grid gap-0.5">
          <h4 className="text-sm font-semibold">{title}</h4>
          {description ? <p className="text-sm text-muted-foreground">{description}</p> : null}
        </div>
        {actions}
      </div>
      {children}
    </div>
  )
}
