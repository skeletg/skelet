import { TrashIcon, PlusIcon } from 'lucide-react'

import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Button } from '@/components/ui/button'

// Walks a subset of JSON Schema Draft-7 and renders an editable form - plugins ship
// condition/action types whose ParamsSchema is rendered here, without the dashboard
// knowing the slug
//
// Supported: type (object|string|integer|number|boolean|array), properties, items, enum,
// required, minimum/maximum, minLength/maxLength, title, description, and the custom
// x-widget dispatch below
//
// Anything else is best-effort - unknown widgets fall back to a JSON textarea, unknown
// types render an info row

export type JSONSchema = {
  type?: string
  properties?: Record<string, JSONSchema>
  items?: JSONSchema
  enum?: Array<string | number>
  required?: string[]
  minimum?: number
  maximum?: number
  minLength?: number
  maxLength?: number
  minItems?: number
  format?: string
  title?: string
  description?: string
  // Skelet custom keyword - falls through to the type's standard widget when unregistered
  'x-widget'?: string
  additionalProperties?: boolean
}

export type WidgetProps = {
  schema: JSONSchema
  value: unknown
  onChange: (v: unknown) => void
  required?: boolean
  // Widgets that render their own label can ignore it
  label?: string
}

export type WidgetRegistry = Record<string, React.ComponentType<WidgetProps>>

// Falls back to title-casing the key when the schema gave no title
function labelFor(key: string, schema: JSONSchema): string {
  if (schema.title) return schema.title
  return key
    .replaceAll('_', ' ')
    .replace(/^./, (c) => c.toUpperCase())
}

// Public entry point - takes the value, its schema, an onChange, and an optional
// widget registry for x-widget dispatch
export function SchemaForm({
  schema,
  value,
  onChange,
  widgets,
}: {
  schema: JSONSchema
  value: unknown
  onChange: (v: unknown) => void
  widgets?: WidgetRegistry
}) {
  const registry = widgets ?? {}
  return <FieldRenderer schema={schema} value={value} onChange={onChange} widgets={registry} />
}

function FieldRenderer({
  schema,
  value,
  onChange,
  widgets,
  label,
  required,
}: {
  schema: JSONSchema
  value: unknown
  onChange: (v: unknown) => void
  widgets: WidgetRegistry
  label?: string
  required?: boolean
}) {
  // Overrides everything below, so a plugin shipping a unique x-widget needs no change here
  const widgetSlug = schema['x-widget']
  if (widgetSlug && widgets[widgetSlug]) {
    const Widget = widgets[widgetSlug]
    return <Widget schema={schema} value={value} onChange={onChange} required={required} label={label} />
  }

  // Object: recursively render each property
  if (schema.type === 'object' && schema.properties) {
    const obj =
      value && typeof value === 'object' && !Array.isArray(value)
        ? (value as Record<string, unknown>)
        : {}
    const requiredSet = new Set(schema.required ?? [])
    return (
      <div className="grid gap-3">
        {Object.entries(schema.properties).map(([key, subSchema]) => (
          <div key={key} className="grid gap-1.5">
            <Label className="flex items-center gap-1">
              {labelFor(key, subSchema)}
              {requiredSet.has(key) ? <span className="text-destructive">*</span> : null}
            </Label>
            <FieldRenderer
              schema={subSchema}
              value={obj[key]}
              onChange={(next) => {
                const copy = { ...obj }
                if (next === undefined || next === '' || (Array.isArray(next) && next.length === 0)) {
                  delete copy[key]
                } else {
                  copy[key] = next
                }
                onChange(copy)
              }}
              widgets={widgets}
              label={labelFor(key, subSchema)}
              required={requiredSet.has(key)}
            />
            {subSchema.description ? (
              <p className="text-xs text-muted-foreground">{subSchema.description}</p>
            ) : null}
          </div>
        ))}
      </div>
    )
  }

  // Array - an enum in `items` renders a multi-select checkbox group, anything else
  // renders add/remove rows with the item schema per row
  if (schema.type === 'array' && schema.items) {
    return <ArrayField schema={schema} value={value} onChange={onChange} widgets={widgets} label={label} />
  }

  // Enum (when not wrapped in an array): Select
  if (schema.enum && schema.enum.length > 0) {
    const current = value == null ? '' : String(value)
    return (
      <Select value={current || undefined} onValueChange={(v) => onChange(coerceFromString(v, schema))}>
        <SelectTrigger>
          <SelectValue placeholder="Select…" />
        </SelectTrigger>
        <SelectContent>
          {schema.enum.map((opt) => (
            <SelectItem key={String(opt)} value={String(opt)}>
              {String(opt)}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    )
  }

  // Boolean: Switch
  if (schema.type === 'boolean') {
    return (
      <Switch
        checked={Boolean(value)}
        onCheckedChange={onChange}
      />
    )
  }

  // Integer / number: number input
  if (schema.type === 'integer' || schema.type === 'number') {
    return (
      <Input
        type="number"
        value={value == null ? '' : String(value)}
        min={schema.minimum}
        max={schema.maximum}
        onChange={(e) => {
          const raw = e.target.value
          if (raw === '') {
            onChange(undefined)
            return
          }
          const parsed = schema.type === 'integer' ? parseInt(raw, 10) : parseFloat(raw)
          onChange(Number.isNaN(parsed) ? undefined : parsed)
        }}
      />
    )
  }

  // String: text input. format=textarea gets a multiline editor
  if (schema.type === 'string' || schema.type === undefined) {
    if (schema.format === 'textarea') {
      return (
        <textarea
          className="min-h-[80px] rounded-md border bg-background px-3 py-2 text-sm"
          value={value == null ? '' : String(value)}
          minLength={schema.minLength}
          maxLength={schema.maxLength}
          onChange={(e) => onChange(e.target.value || undefined)}
        />
      )
    }
    return (
      <Input
        value={value == null ? '' : String(value)}
        minLength={schema.minLength}
        maxLength={schema.maxLength}
        onChange={(e) => onChange(e.target.value || undefined)}
      />
    )
  }

  // Unknown type - raw JSON fallback so the value stays editable; valid built-in schemas never hit this
  return (
    <p className="text-xs text-muted-foreground">
      Unsupported schema type: <code>{schema.type ?? 'unknown'}</code>
    </p>
  )
}

function ArrayField({
  schema,
  value,
  onChange,
  widgets,
  label,
}: {
  schema: JSONSchema
  value: unknown
  onChange: (v: unknown) => void
  widgets: WidgetRegistry
  label?: string
}) {
  const items = Array.isArray(value) ? (value as unknown[]) : []
  const itemSchema = schema.items as JSONSchema

  // Inline chip list rather than a menu, so every option is visible at once
  if (itemSchema.enum && itemSchema.enum.length > 0) {
    const selected = new Set(items.map((v) => String(v)))
    return (
      <div className="grid grid-cols-2 gap-2">
        {itemSchema.enum.map((opt) => {
          const optStr = String(opt)
          const checked = selected.has(optStr)
          return (
            <label
              key={optStr}
              className="flex cursor-pointer items-center gap-2 rounded border px-3 py-1.5 text-sm"
            >
              <Checkbox
                checked={checked}
                onCheckedChange={(c) => {
                  const next = new Set(selected)
                  if (c) next.add(optStr)
                  else next.delete(optStr)
                  onChange(Array.from(next).map((s) => coerceFromString(s, itemSchema)))
                }}
              />
              <span>{optStr}</span>
            </label>
          )
        })}
      </div>
    )
  }

  // Generic array: add/remove rows
  const setAt = (idx: number, next: unknown) => {
    const copy = items.slice()
    if (next === undefined || next === '') {
      copy.splice(idx, 1)
    } else {
      copy[idx] = next
    }
    onChange(copy)
  }
  const removeAt = (idx: number) => {
    const copy = items.slice()
    copy.splice(idx, 1)
    onChange(copy)
  }
  const add = () => {
    const empty = defaultForSchema(itemSchema)
    onChange([...items, empty])
  }

  return (
    <div className="grid gap-2">
      {items.length === 0 ? (
        <p className="text-xs text-muted-foreground">No entries.</p>
      ) : (
        items.map((item, idx) => (
          <div key={idx} className="flex items-start gap-2">
            <div className="flex-1">
              <FieldRenderer
                schema={itemSchema}
                value={item}
                onChange={(next) => setAt(idx, next)}
                widgets={widgets}
                label={label ? `${label} #${idx + 1}` : undefined}
              />
            </div>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              onClick={() => removeAt(idx)}
              aria-label="Remove"
            >
              <TrashIcon className="size-4" />
            </Button>
          </div>
        ))
      )}
      <Button type="button" variant="outline" size="sm" onClick={add} className="w-fit">
        <PlusIcon className="size-4" /> Add
      </Button>
    </div>
  )
}

// An empty value matching the schema, for the "Add" button on an array - gives the new
// row a type-correct start so the next FieldRenderer render never sees undefined
function defaultForSchema(schema: JSONSchema): unknown {
  if (schema.type === 'object') return {}
  if (schema.type === 'array') return []
  if (schema.type === 'boolean') return false
  if (schema.type === 'integer' || schema.type === 'number') return 0
  return ''
}

// enum entries are stored as numbers when the schema says so, and without coercion a
// Select's string value would persist "1" instead of 1
function coerceFromString(s: string, schema: JSONSchema): unknown {
  if (schema.type === 'integer') return parseInt(s, 10)
  if (schema.type === 'number') return parseFloat(s)
  return s
}

