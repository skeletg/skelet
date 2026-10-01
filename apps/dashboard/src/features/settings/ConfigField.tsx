import type { ChangeEvent } from 'react'
import { RotateCcwIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { cn } from '@/lib/utils'
import type { ConfigScoping, ConfigType } from '@/graphql/schema.types'
import { Highlight } from './Highlight'
import { MultiselectEditor } from './MultiselectEditor'
import { SearchAttributesEditor } from './SearchAttributesEditor'
import { SortOptionsEditor } from './SortOptionsEditor'
import { WorkingHoursEditor } from './WorkingHoursEditor'
import { ConfigImageField } from './ConfigImageField'
import { ConfigColorField } from './ConfigColorField'
import { isValidJson } from './useConfigDrafts'

export type ConfigFieldData = {
  path: string
  section: string
  group: string
  field: string
  label: string
  /** Registry help text; empty until one is written, in which case the label stands alone */
  description?: string | null
  type: ConfigType
  value: string
  // SECRET fields: value is always "" (never returned); isSet says whether one
  // is configured server-side
  isSet: boolean
  options?: { label: string; value: string }[] | null
  isDefault: boolean
  public: boolean
  scoping: ConfigScoping
  scope: {
    kind: string
    channelId?: string | null
    locale?: string | null
  }
}

// Only a staged edit marks itself at rest - where a value came from is answered on hover, since a
// permanent mark on every row holding an override reads as "changed" and turns a long list into stripes
// Left edge only - a bare `border-<color>` would repaint the divide-y rule between rows too
const RAIL = {
  dirty: 'border-l-amber-500',
  quiet: 'border-l-transparent',
} as const

// A registry description is the real help text. Where none is written yet the label convention
// stands in: labels park units and qualifiers in a trailing paren - "Minimum order amount
// (cents)" - so lift that onto the hint line rather than leaving it in the label
// Retire the paren fallback once every field carries a description
function splitLabel(label: string, description?: string | null): { text: string; hint?: string } {
  const desc = description?.trim()
  const match = /^(.{3,}?)\s*\(([^()]{1,60})\)$/.exec(label.trim())
  if (desc) {
    return { text: match ? match[1] : label, hint: desc }
  }
  if (!match) return { text: label }
  return { text: match[1], hint: match[2] }
}

export function ConfigField({
  field,
  draft,
  onChange,
  onReset,
  resetting,
  disabled,
  inherited,
  scopeLabel = 'the platform defaults',
  lockedReason,
  highlight = [],
}: {
  field: ConfigFieldData
  draft: string | undefined
  onChange: (value: string | undefined) => void
  onReset: () => void
  resetting?: boolean
  disabled?: boolean
  /** Omit on surfaces that don't model scope - the row then shows no provenance at all */
  inherited?: boolean
  /** Names what an inherited value falls back to, e.g. "the platform defaults" */
  scopeLabel?: string
  /** Set when the header scope can't hold this knob - the row goes read-only and says why */
  lockedReason?: string
  /** Active search terms, marked up in the label and path so a hit is findable in a long list */
  highlight?: string[]
}) {
  const locked = !!lockedReason
  const lockNote = lockedReason?.trim() ? lockedReason : ''  
  const current = draft ?? field.value
  const isDirty = draft !== undefined && draft !== field.value
  const jsonInvalid = field.type === 'JSON' && isDirty && !isValidJson(current)
  const { text, hint } = splitLabel(field.label, field.description)

  const provenance = isDirty
    ? { rail: RAIL.dirty, note: 'Unsaved', tone: 'text-warning', onHover: false }
    : inherited === undefined
      ? { rail: RAIL.quiet, note: '', tone: '', onHover: true }
      : inherited
        ? {
            rail: RAIL.quiet,
            note: field.isDefault ? 'Built-in default' : `Inherited from ${scopeLabel}`,
            tone: 'text-muted-foreground',
            onHover: true,
          }
        : { rail: RAIL.quiet, note: 'Set here', tone: 'text-muted-foreground', onHover: true }

  return (
    <div
      className={cn(
        'group grid items-start gap-x-8 gap-y-2 border-l-2 py-3 pl-5 pr-6 transition-colors hover:bg-muted/30 md:grid-cols-[minmax(0,20rem)_minmax(0,1fr)]',
        provenance.rail,
      )}
    >
      <div className="min-w-0">
        <Label
          htmlFor={`cfg-${field.path}`}
          className="block cursor-pointer text-sm font-medium leading-5"
        >
          <Highlight text={text} terms={highlight} />
        </Label>
        {hint && (
          <p className="mt-0.5 text-xs text-muted-foreground">
            <Highlight text={hint} terms={highlight} />
          </p>
        )}
        {/* Marker row: what the storefront can read stays visible, the path is a hover away */}
        <p className="mt-1 flex min-h-4 min-w-0 items-baseline gap-2 text-[10px]">
          {field.public && (
            <span
              className="shrink-0 text-muted-foreground/60"
              title="Readable by the storefront through publicConfig"
            >
              public
            </span>
          )}
          <span
            className={cn(
              'truncate font-mono text-muted-foreground/60 transition-opacity group-focus-within:opacity-100 group-hover:opacity-100',
              // A search can match the path, so keep it visible while one is running
              highlight.length === 0 && 'opacity-0',
            )}
          >
            <Highlight text={field.path} terms={highlight} />
          </span>
        </p>
      </div>

      <div className="flex min-w-0 items-start gap-2">
        <div className="min-w-0 flex-1">
          <FieldEditor
            id={`cfg-${field.path}`}
            type={field.type}
            value={current}
            isSet={field.isSet}
            options={field.options ?? []}
            onChange={(v) => onChange(v === field.value ? undefined : v)}
            disabled={disabled || locked}
            invalid={jsonInvalid}
            path={field.path}
          />
          <div className="mt-1 flex min-h-4 flex-wrap items-center gap-2 text-[10px]">
            {locked ? (
              lockNote ? <span className="text-muted-foreground">{lockNote}</span> : null
            ) : jsonInvalid ? (
              <span className="font-medium text-destructive">Not valid JSON</span>
            ) : provenance.note ? (
              <span
                className={cn(
                  provenance.tone,
                  provenance.onHover &&
                    'opacity-0 transition-opacity group-focus-within:opacity-100 group-hover:opacity-100',
                )}
              >
                {provenance.note}
              </span>
            ) : null}
          </div>
        </div>
        {/* Reserved whether or not the button is there, so controls don't shift width row to row */}
        <div className="w-8 shrink-0">
          {!field.isDefault && !locked && (
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              onClick={onReset}
              disabled={resetting || disabled}
              title={
                inherited
                  ? `Clear this value and follow ${scopeLabel}`
                  : 'Restore the inherited value'
              }
              className="size-8 text-muted-foreground opacity-0 transition-opacity hover:text-foreground group-focus-within:opacity-100 group-hover:opacity-100"
            >
              <RotateCcwIcon className="size-3.5" />
            </Button>
          )}
        </div>
      </div>
    </div>
  )
}

// Config fields describe the STORE, never the admin filling them in, so no browser or password manager should ever offer a saved value here
// - "Support email" was getting the operator's own address dropped into it, which staged a change they never made
const NO_AUTOFILL = {
  autoComplete: 'off',
  'data-1p-ignore': true,
  'data-lpignore': 'true',
  'data-form-type': 'other',
} as const

// Belt to that braces: an automatic fill writes into a field without focusing it, while typing,
// pasting and picking a suggestion all focus first - so an unfocused change event is not an edit
// The box is snapped back to the stored value too, or it would sit there showing text that isn't
// staged and prefix whatever the admin types next
function textChange(value: string, onChange: (v: string) => void) {
  return (e: ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => {
    if (document.activeElement !== e.currentTarget) {
      e.currentTarget.value = value
      return
    }
    onChange(e.target.value)
  }
}

// One width per kind of answer, so a column of controls lines up instead of every input taking
// whatever the container gives it - a phone number and a JSON blob shouldn't measure the same
const W = {
  number: 'max-w-[11rem]',
  short: 'max-w-[16rem]',
  text: 'max-w-md',
  block: 'max-w-2xl',
} as const

function FieldEditor({
  id,
  type,
  value,
  isSet,
  options,
  onChange,
  disabled,
  invalid,
  path,
}: {
  id: string
  type: ConfigType
  value: string
  isSet?: boolean
  options: { label: string; value: string }[]
  onChange: (v: string) => void
  disabled?: boolean
  invalid?: boolean
  path?: string
}) {
  switch (type) {
    case 'SECRET':
      // Write-only: the value is never returned, so the input starts empty
      // Typing replaces the stored secret; leaving it blank keeps the current
      // one (the save path skips empty secrets)
      //
      // 'new-password', not 'off': Chrome ignores `off` on password inputs and runs credential
      // autofill anyway, which fills a nearby text box with the admin's saved username
      return (
        <Input
          id={id}
          type="password"
          {...NO_AUTOFILL}
          autoComplete="new-password"
          value={value}
          onChange={textChange(value, onChange)}
          disabled={disabled}
          placeholder={isSet ? '•••••••• configured - type to replace' : 'Not set'}
          className={W.text}
        />
      )

    case 'BOOL':
      return (
        <div className="flex h-9 items-center">
          <Switch
            id={id}
            checked={value === 'true'}
            onCheckedChange={(v) => onChange(v ? 'true' : 'false')}
            disabled={disabled}
          />
          <span className="ml-3 text-sm text-muted-foreground">
            {value === 'true' ? 'Enabled' : 'Disabled'}
          </span>
        </div>
      )

    case 'INT':
      return (
        <Input
          id={id}
          type="number"
          inputMode="numeric"
          {...NO_AUTOFILL}
          value={value}
          onChange={textChange(value, onChange)}
          disabled={disabled}
          className={W.number}
        />
      )

    case 'PHONE':
      // Saved as E.164 - a store-wide setting has no country to read a national number in
      return (
        <Input
          id={id}
          type="tel"
          placeholder="+ country code and number"
          {...NO_AUTOFILL}
          value={value}
          onChange={textChange(value, onChange)}
          disabled={disabled}
          className={W.short}
        />
      )

    case 'TEXTAREA':
      return (
        <Textarea
          id={id}
          {...NO_AUTOFILL}
          value={value}
          onChange={textChange(value, onChange)}
          disabled={disabled}
          rows={3}
          className={W.block}
        />
      )

    case 'SELECT':
      return (
        <Select value={value} onValueChange={onChange} disabled={disabled}>
          <SelectTrigger id={id} className={cn('w-full', W.short)}>
            <SelectValue placeholder="Select an option..." />
          </SelectTrigger>
          <SelectContent>
            {options.map((opt) => (
              <SelectItem key={opt.value} value={opt.value}>
                {opt.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      )

    case 'MULTISELECT':
      return (
        <div className={W.block}>
          <MultiselectEditor options={options} value={value} onChange={onChange} disabled={disabled} />
        </div>
      )

    case 'IMAGE':
      return (
        <div className={W.text}>
          <ConfigImageField value={value} onChange={onChange} disabled={disabled} path={path} />
        </div>
      )

    case 'COLOR':
      return (
        <div className={W.short}>
          <ConfigColorField value={value} onChange={onChange} disabled={disabled} />
        </div>
      )

    case 'WORKING_HOURS':
      return (
        <WorkingHoursEditor value={value} onChange={onChange} disabled={disabled} />
      )

    case 'SEARCH_ATTRIBUTES':
      return (
        <SearchAttributesEditor value={value} onChange={onChange} disabled={disabled} path={path} />
      )

    case 'SORT_OPTIONS':
      return (
        <SortOptionsEditor value={value} onChange={onChange} disabled={disabled} />
      )

    case 'JSON':
      return (
        <Textarea
          id={id}
          {...NO_AUTOFILL}
          value={value}
          onChange={textChange(value, onChange)}
          disabled={disabled}
          rows={5}
          className={cn('font-mono text-xs', W.block, invalid && 'border-destructive')}
        />
      )

    default:
      return (
        <Input
          id={id}
          {...NO_AUTOFILL}
          value={value}
          onChange={textChange(value, onChange)}
          disabled={disabled}
          className={W.text}
        />
      )
  }
}

