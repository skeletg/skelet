import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import {
  FormRichTextEditor,
  type RichTextValue,
} from '@/features/catalog/LexicalRichTextEditor'
import type { CmsInputType } from '@/graphql/schema.types'
import { AssetField, AssetsField } from './AssetAttributeField'
import type { CmsAttributeRowFragment } from './Cms.gql'

// FieldValue is the canonical in-form representation of a single attribute
// value before it ships back to the API - Selectable / relationship values
// don't pass through here - they live in the page's selections + relationships
// arrays instead - This component handles the scalar/JSON/asset families
export type FieldValue =
  | string
  | number
  | boolean
  | RichTextValue
  | string[]
  | null
  | undefined
  | unknown

export function DynamicField({
  attribute,
  isRequired,
  disabled,
  value,
  onChange,
}: {
  attribute: CmsAttributeRowFragment
  isRequired: boolean
  disabled?: boolean
  value: FieldValue
  onChange: (next: FieldValue) => void
}) {
  const label = attribute.content?.name ?? attribute.slug
  const helperText = attribute.content?.helperText
  const placeholder = attribute.content?.placeholder ?? undefined
  const fieldId = `cms-field-${attribute.id}`

  return (
    <div className="grid gap-1.5">
      <Label htmlFor={fieldId} className="flex items-center gap-1">
        {label}
        {isRequired && <span className="text-destructive">*</span>}
        <code className="ml-1 text-xs font-normal text-muted-foreground">{attribute.slug}</code>
      </Label>
      <FieldInput
        id={fieldId}
        type={attribute.inputType}
        placeholder={placeholder}
        disabled={disabled}
        value={value}
        onChange={onChange}
        unit={attribute.unit ?? undefined}
      />
      {helperText && <p className="text-xs text-muted-foreground">{helperText}</p>}
    </div>
  )
}

function FieldInput({
  id,
  type,
  placeholder,
  disabled,
  value,
  onChange,
  unit,
}: {
  id: string
  type: CmsInputType
  placeholder?: string
  disabled?: boolean
  value: FieldValue
  onChange: (v: FieldValue) => void
  unit?: string
}) {
  switch (type) {
    case 'TEXT':
    case 'URL':
      return (
        <Input
          id={id}
          value={typeof value === 'string' ? value : ''}
          onChange={(e) => onChange(e.target.value)}
          placeholder={placeholder}
          disabled={disabled}
        />
      )
    case 'TEXTAREA':
      return (
        <Textarea
          id={id}
          value={typeof value === 'string' ? value : ''}
          onChange={(e) => onChange(e.target.value)}
          placeholder={placeholder}
          rows={4}
          disabled={disabled}
        />
      )
    case 'RICHTEXT':
      return (
        <FormRichTextEditor
          value={(value as RichTextValue) ?? undefined}
          fallbackText=""
          disabled={disabled}
          placeholder={placeholder ?? 'Write…'}
          onChange={(v) => onChange(v)}
        />
      )
    case 'NUMBER': {
      const numStr = typeof value === 'number' ? String(value) : typeof value === 'string' ? value : ''
      return (
        <div className="flex items-center gap-2">
          <Input
            id={id}
            type="number"
            value={numStr}
            onChange={(e) => onChange(e.target.value === '' ? null : Number(e.target.value))}
            placeholder={placeholder}
            disabled={disabled}
          />
          {unit && <span className="text-sm text-muted-foreground">{unit}</span>}
        </div>
      )
    }
    case 'BOOLEAN':
      return (
        <Switch
          id={id}
          checked={value === true}
          onCheckedChange={(v) => onChange(v)}
          disabled={disabled}
        />
      )
    case 'DATE':
      return (
        <Input
          id={id}
          type="date"
          value={typeof value === 'string' ? value : ''}
          onChange={(e) => onChange(e.target.value)}
          disabled={disabled}
        />
      )
    case 'DATETIME':
      return (
        <Input
          id={id}
          type="datetime-local"
          value={typeof value === 'string' ? value : ''}
          onChange={(e) => onChange(e.target.value)}
          disabled={disabled}
        />
      )
    case 'ASSET':
      return (
        <AssetField
          value={typeof value === 'string' ? value : ''}
          onChange={(v) => onChange(v)}
          disabled={disabled}
        />
      )
    case 'ASSETS': {
      const arr = Array.isArray(value) ? (value as string[]) : []
      return <AssetsField value={arr} onChange={(v) => onChange(v)} disabled={disabled} />
    }
    case 'JSON': {
      const txt = value === undefined || value === null ? '' : JSON.stringify(value, null, 2)
      return (
        <Textarea
          id={id}
          value={txt}
          onChange={(e) => {
            const raw = e.target.value
            if (!raw.trim()) {
              onChange(null)
              return
            }
            try {
              onChange(JSON.parse(raw))
            } catch {
              onChange(raw) // keep raw text in state until valid; service will reject if still invalid
            }
          }}
          placeholder={placeholder ?? '{}'}
          rows={6}
          className="font-mono text-xs"
          disabled={disabled}
        />
      )
    }
    case 'SELECT':
    case 'MULTISELECT':
    case 'RELATIONSHIP':
      return (
        <p className="rounded-md border bg-muted/40 px-2 py-2 text-xs text-muted-foreground">
          Edit {type.toLowerCase()} values via the dedicated picker below.
        </p>
      )
  }
  return (
    <Input
      id={id}
      value={typeof value === 'string' ? value : ''}
      onChange={(e) => onChange(e.target.value)}
      disabled={disabled}
    />
  )
}
