import type { ComponentProps, ReactNode } from 'react'
import type { AnyFieldApi } from '@tanstack/react-form'
import { PlusIcon, Trash2Icon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { useFieldContext } from './form-context'
import { fieldErrorText } from './field-errors'

// The dashboard's bound fields - rendered as <form.AppField name="x">{(f) => <f.TextField … />}</form.AppField>
// so a page names each field once and gets its label, control and error wired together

export function FieldError({ field }: { field: AnyFieldApi }) {
  const text = fieldErrorText(field)
  if (!text) return null
  return <p className="text-xs text-destructive">{text}</p>
}

type TextFieldProps = Omit<ComponentProps<typeof Input>, 'value' | 'onChange' | 'onBlur' | 'name'> & {
  label?: ReactNode
  hint?: ReactNode
  /** Extra note under the control - usePersistedForm's clash note goes here */
  note?: ReactNode
  /** Runs after the value is staged - for fields that drive another (name -> slug) */
  onValueChange?: (value: string) => void
  /** Normalises what's typed before it's staged - uppercasing an ISO code, say */
  parse?: (raw: string) => string
}

export function TextField({ label, hint, note, id, className, onValueChange, parse, ...inputProps }: TextFieldProps) {
  const field = useFieldContext<string>()
  const inputId = id ?? `f-${field.name}`
  const error = fieldErrorText(field)
  return (
    <div className={['grid gap-1.5', className].filter(Boolean).join(' ')}>
      {label && <Label htmlFor={inputId}>{label}</Label>}
      <Input
        {...inputProps}
        id={inputId}
        name={field.name}
        value={field.state.value}
        aria-invalid={error ? true : undefined}
        onChange={(e) => {
          const value = parse ? parse(e.target.value) : e.target.value
          field.handleChange(value)
          onValueChange?.(value)
        }}
        onBlur={field.handleBlur}
      />
      {error ? (
        <p className="text-xs text-destructive">{error}</p>
      ) : (
        hint && <p className="text-xs text-muted-foreground">{hint}</p>
      )}
      {note}
    </div>
  )
}

type TextareaFieldProps = Omit<ComponentProps<typeof Textarea>, 'value' | 'onChange' | 'onBlur' | 'name'> & {
  label?: ReactNode
  hint?: ReactNode
  note?: ReactNode
}

export function TextareaField({ label, hint, note, id, className, ...props }: TextareaFieldProps) {
  const field = useFieldContext<string>()
  const inputId = id ?? `f-${field.name}`
  const error = fieldErrorText(field)
  return (
    <div className={['grid gap-1.5', className].filter(Boolean).join(' ')}>
      {label && <Label htmlFor={inputId}>{label}</Label>}
      <Textarea
        {...props}
        id={inputId}
        name={field.name}
        value={field.state.value}
        aria-invalid={error ? true : undefined}
        onChange={(e) => field.handleChange(e.target.value)}
        onBlur={field.handleBlur}
      />
      {error ? (
        <p className="text-xs text-destructive">{error}</p>
      ) : (
        hint && <p className="text-xs text-muted-foreground">{hint}</p>
      )}
      {note}
    </div>
  )
}

type NumberFieldProps = Omit<ComponentProps<typeof Input>, 'value' | 'onChange' | 'onBlur' | 'name' | 'type'> & {
  label?: ReactNode
  hint?: ReactNode
  note?: ReactNode
}

// A whole-number input bound to a number value - an emptied box reads as 0, like the pages always did
export function NumberField({ label, hint, note, id, className, ...inputProps }: NumberFieldProps) {
  const field = useFieldContext<number>()
  const inputId = id ?? `f-${field.name}`
  const error = fieldErrorText(field)
  return (
    <div className={['grid gap-1.5', className].filter(Boolean).join(' ')}>
      {label && <Label htmlFor={inputId}>{label}</Label>}
      <Input
        inputMode="numeric"
        {...inputProps}
        type="number"
        id={inputId}
        name={field.name}
        value={field.state.value}
        aria-invalid={error ? true : undefined}
        onChange={(e) => field.handleChange(Number(e.target.value) || 0)}
        onBlur={field.handleBlur}
      />
      {error ? (
        <p className="text-xs text-destructive">{error}</p>
      ) : (
        hint && <p className="text-xs text-muted-foreground">{hint}</p>
      )}
      {note}
    </div>
  )
}

export type SelectOption = { value: string; label: ReactNode }

const NONE = '__none__'

// A string field picked from a list. `noneLabel` adds a first entry that stands for '' - Radix
// Select can't hold an empty value itself, so '' round-trips through a sentinel
export function SelectField({
  label,
  id,
  options,
  placeholder,
  noneLabel,
  hint,
  note,
  disabled,
  className,
}: {
  label?: ReactNode
  id?: string
  options: SelectOption[]
  placeholder?: string
  noneLabel?: ReactNode
  hint?: ReactNode
  note?: ReactNode
  disabled?: boolean
  className?: string
}) {
  const field = useFieldContext<string>()
  const selectId = id ?? `f-${field.name}`
  const error = fieldErrorText(field)
  const value = field.state.value === '' && noneLabel !== undefined ? NONE : field.state.value
  return (
    <div className={['grid gap-1.5', className].filter(Boolean).join(' ')}>
      {label && <Label htmlFor={selectId}>{label}</Label>}
      <Select
        value={value}
        disabled={disabled}
        onValueChange={(v) => {
          field.handleChange(v === NONE ? '' : v)
          field.handleBlur()
        }}
      >
        <SelectTrigger id={selectId} className="w-full" aria-invalid={error ? true : undefined}>
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
      {error ? (
        <p className="text-xs text-destructive">{error}</p>
      ) : (
        hint && <p className="text-xs text-muted-foreground">{hint}</p>
      )}
      {note}
    </div>
  )
}

type MetadataRow = { key: string; value: string }

// Free-form key/value rows (shared/schemas/metadata) - edits the whole array in one go, which keeps
// the diff and the stored draft working on the field as one value
export function MetadataField({ note }: { note?: ReactNode }) {
  const field = useFieldContext<MetadataRow[]>()
  const rows = field.state.value
  const set = (next: MetadataRow[]) => field.handleChange(next)
  return (
    <div className="grid gap-2">
      {rows.length === 0 ? (
        <p className="text-sm text-muted-foreground">No metadata yet.</p>
      ) : (
        rows.map((row, i) => (
          <div key={i} className="grid grid-cols-[1fr_2fr_auto] items-center gap-2">
            <Input
              placeholder="Key"
              aria-label={`Metadata ${i + 1} key`}
              value={row.key}
              onChange={(e) => set(rows.map((r, j) => (j === i ? { ...r, key: e.target.value } : r)))}
            />
            <Input
              placeholder="Value"
              aria-label={`Metadata ${i + 1} value`}
              value={row.value}
              onChange={(e) => set(rows.map((r, j) => (j === i ? { ...r, value: e.target.value } : r)))}
            />
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="size-8"
              aria-label={`Remove metadata ${i + 1}`}
              onClick={() => set(rows.filter((_, j) => j !== i))}
            >
              <Trash2Icon className="size-4" />
            </Button>
          </div>
        ))
      )}
      {note}
      <Button
        type="button"
        variant="outline"
        size="sm"
        className="w-fit"
        onClick={() => set([...rows, { key: '', value: '' }])}
      >
        <PlusIcon className="size-4" />
        Add row
      </Button>
    </div>
  )
}

// A checkbox row - for flags that read as a sentence ("Active - inactive countries can't …")
export function CheckboxField({ label, id }: { label: ReactNode; id?: string }) {
  const field = useFieldContext<boolean>()
  const boxId = id ?? `f-${field.name}`
  return (
    <div className="flex items-center gap-2">
      <Checkbox id={boxId} checked={field.state.value} onCheckedChange={(v) => field.handleChange(!!v)} />
      <Label htmlFor={boxId} className="cursor-pointer">{label}</Label>
    </div>
  )
}

// The bordered toggle row the detail pages use - label and description left, switch right
export function SwitchField({
  label,
  description,
  id,
  note,
}: {
  label: ReactNode
  description?: ReactNode
  id?: string
  note?: ReactNode
}) {
  const field = useFieldContext<boolean>()
  const switchId = id ?? `f-${field.name}`
  return (
    <div className="flex items-start justify-between gap-4 rounded-md border p-4">
      <div className="grid gap-0.5">
        <Label htmlFor={switchId} className="text-sm">{label}</Label>
        {description && <p className="text-xs text-muted-foreground">{description}</p>}
        {note}
      </div>
      <Switch id={switchId} checked={field.state.value} onCheckedChange={field.handleChange} />
    </div>
  )
}
