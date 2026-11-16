import { useRef, type ComponentProps, type ReactNode } from 'react'
import type { AnyFieldApi } from '@tanstack/react-form'
import { PlusIcon, Trash2Icon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { formatAsTyped, formatPhone } from '@/lib/phone'
import { getDetailFields } from '@/plugins/field-registry'
import { useFieldContext } from './form-context'
import { PluginMetadataFields } from './PluginMetadataFields'
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

type PhoneFieldProps = Omit<TextFieldProps, 'type' | 'parse' | 'onValueChange' | 'defaultValue'> & {
  /** The country a nationally typed number is formatted in - the address's own, when there is one */
  country?: string
}

// A TextField that formats as it's typed. It only reformats while typing at the end - mid-edit the caret would jump,
// and reformatting a deletion would put back the bracket or space just removed. A stored E.164 value shows
// international until it's edited
export function PhoneField({ label, hint, note, id, className, country, ...inputProps }: PhoneFieldProps) {
  const field = useFieldContext<string>()
  const inputId = id ?? `f-${field.name}`
  const error = fieldErrorText(field)
  const value = field.state.meta.isDirty ? field.state.value : formatPhone(field.state.value)
  return (
    <div className={['grid gap-1.5', className].filter(Boolean).join(' ')}>
      {label && <Label htmlFor={inputId}>{label}</Label>}
      <Input
        autoComplete="tel"
        {...inputProps}
        type="tel"
        inputMode="tel"
        id={inputId}
        name={field.name}
        value={value}
        aria-invalid={error ? true : undefined}
        onChange={(e) => {
          const el = e.target
          const inputType = (e.nativeEvent as InputEvent).inputType ?? ''
          const atEnd = el.selectionStart === el.value.length
          field.handleChange(atEnd && !inputType.startsWith('delete') ? formatAsTyped(el.value, country) : el.value)
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

export type SelectOption = { value: string; label: ReactNode; disabled?: boolean }

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
            <SelectItem key={o.value} value={o.value} disabled={o.disabled}>
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

// Which rows the plugin fields own: on a value loaded or reset from outside, the first row with each claimed key, and
// afterwards the rows the fields themselves write. A free row typed into a claimed key stays free, flagged rather than
// vanishing mid-word
function usePluginRows(rows: MetadataRow[], claimed: Set<string>) {
  const owned = useRef(new WeakSet<MetadataRow>())
  const written = useRef<MetadataRow[] | null>(null)
  if (rows !== written.current) {
    const fresh = new WeakSet<MetadataRow>()
    const taken = new Set<string>()
    for (const r of rows) {
      const k = r.key.trim()
      if (claimed.has(k) && !taken.has(k)) {
        fresh.add(r)
        taken.add(k)
      }
    }
    owned.current = fresh
  }
  return {
    owns: (r: MetadataRow) => owned.current.has(r),
    own: (r: MetadataRow) => owned.current.add(r),
    wrote: (next: MetadataRow[], commit: (next: MetadataRow[]) => void) => {
      written.current = next
      commit(next)
    },
  }
}

// Free-form key/value rows (shared/schemas/metadata) - edits the whole array in one go, which keeps
// the diff and the stored draft working on the field as one value
// `label` names the rows for screen readers - pages with two lists (public, private) tell them apart
export function MetadataField({ note, label = 'Metadata', target }: { note?: ReactNode; label?: string; target?: string }) {
  const field = useFieldContext<MetadataRow[]>()
  const rows = field.state.value
  const error = fieldErrorText(field)
  const plugin = getDetailFields(target)
  const claimed = new Set(plugin.map((f) => f.key))
  const owned = usePluginRows(rows, claimed)
  const set = (next: MetadataRow[]) => owned.wrote(next, field.handleChange)
  const free = rows.map((row, i) => ({ row, i })).filter(({ row }) => !owned.owns(row))
  return (
    <div className="grid gap-2">
      {plugin.length > 0 && (
        <div className="mb-2 border-b pb-4">
          <PluginMetadataFields fields={plugin} rows={rows} onChange={set} owns={owned.owns} own={owned.own} />
        </div>
      )}
      {free.length === 0 ? (
        <p className="text-sm text-muted-foreground">{plugin.length > 0 ? 'No other metadata.' : 'No metadata yet.'}</p>
      ) : (
        free.map(({ row, i }, n) => (
          <div key={i} className="grid grid-cols-[1fr_2fr_auto] items-center gap-2">
            <Input
              placeholder="Key"
              aria-label={`${label} ${n + 1} key`}
              value={row.key}
              onChange={(e) => set(rows.map((r, j) => (j === i ? { ...r, key: e.target.value } : r)))}
            />
            <Input
              placeholder="Value"
              aria-label={`${label} ${n + 1} value`}
              value={row.value}
              onChange={(e) => set(rows.map((r, j) => (j === i ? { ...r, value: e.target.value } : r)))}
            />
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="size-8"
              aria-label={`Remove ${label.toLowerCase()} ${n + 1}`}
              onClick={() => set(rows.filter((_, j) => j !== i))}
            >
              <Trash2Icon className="size-4" />
            </Button>
            {claimed.has(row.key.trim()) && (
              <p className="col-span-3 -mt-1 text-xs text-destructive">
                {plugin.find((f) => f.key === row.key.trim())?.label} has its own field above - use that, or another key
              </p>
            )}
          </div>
        ))
      )}
      {error && <p className="text-xs text-destructive">{error}</p>}
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
  disabled,
  bordered = true,
}: {
  label: ReactNode
  description?: ReactNode
  id?: string
  note?: ReactNode
  disabled?: boolean
  /** False for rows grouped inside one bordered box */
  bordered?: boolean
}) {
  const field = useFieldContext<boolean>()
  const switchId = id ?? `f-${field.name}`
  const error = fieldErrorText(field)
  return (
    <div className={bordered ? 'flex items-start justify-between gap-4 rounded-md border p-4' : 'flex items-start justify-between gap-4'}>
      <div className="grid gap-0.5">
        <Label htmlFor={switchId} className="text-sm">{label}</Label>
        {description && <p className="text-xs text-muted-foreground">{description}</p>}
        {error && <p className="text-xs text-destructive">{error}</p>}
        {note}
      </div>
      <Switch id={switchId} checked={field.state.value} onCheckedChange={field.handleChange} disabled={disabled} />
    </div>
  )
}
