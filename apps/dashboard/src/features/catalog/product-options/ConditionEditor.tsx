import { PlusIcon, Trash2Icon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { optionContentFor, valueContentFor, type ProductOption, type VisibilityRule } from '../productOptionsStaging'
import { isChoice } from './option-types'

const OP_LABEL: Record<VisibilityRule['op'], string> = {
  anyOf: 'is one of',
  noneOf: 'is not one of',
  filled: 'is filled in',
  empty: 'is left empty',
}

// "Show this option only when Wrapping is one of Gift, Premium" - one sentence per rule, over the options listed above
// this one. Unsaved ones count: the save gives them ids before it writes any condition
export function ConditionEditor({
  earlier,
  isNew,
  forRule = false,
  locale,
  logic,
  rules,
  nested,
  disabled,
  error,
  onChange,
}: {
  earlier: ProductOption[]
  isNew: boolean
  // A rule's condition decides when it applies, not whether an option shows
  forRule?: boolean
  locale: string
  logic: 'and' | 'or'
  rules: VisibilityRule[]
  // Authored through the API in a shape this editor can't show - kept as is
  nested: boolean
  disabled: boolean
  error?: string
  onChange: (next: { logic?: 'and' | 'or'; rules?: VisibilityRule[] }) => void
}) {
  if (earlier.length === 0 && rules.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">
        {isNew
          ? 'Always shown. Once the product has another option, this one can be set to show only for certain answers to it.'
          : 'Always shown - it comes first. A condition can only depend on options listed above it, so move it down to make it depend on one.'}
      </p>
    )
  }
  if (nested) {
    return (
      <p className="text-sm text-muted-foreground">
        This {forRule ? 'rule' : 'option'} has a condition set through the API that can't be edited here. It's kept as it is.
      </p>
    )
  }
  const update = (i: number, patch: Partial<VisibilityRule>) => onChange({ rules: rules.map((r, j) => (j === i ? { ...r, ...patch } : r)) })
  const title = (o: ProductOption) => optionContentFor(o, locale).title || 'Untitled option'

  return (
    <div className="grid gap-3">
      {rules.length === 0 ? (
        <p className="text-sm text-muted-foreground">{forRule ? 'No condition yet.' : 'Always shown.'}</p>
      ) : rules.length > 1 ? (
        <div className="flex flex-wrap items-center gap-2 text-sm">
          <span>{forRule ? 'Applies when' : 'Show it when'}</span>
          <Select value={logic} onValueChange={(v) => onChange({ logic: v as 'and' | 'or' })} disabled={disabled}>
            <SelectTrigger size="sm" className="w-24" aria-label="Which conditions must match"><SelectValue /></SelectTrigger>
            <SelectContent>
              <SelectItem value="and">all</SelectItem>
              <SelectItem value="or">any</SelectItem>
            </SelectContent>
          </Select>
          <span>of these are true:</span>
        </div>
      ) : (
        <p className="text-sm">{forRule ? 'Applies when:' : 'Show it only when:'}</p>
      )}

      {rules.map((rule, i) => {
        const parent = earlier.find((o) => o.id === rule.option_id)
        const ops: VisibilityRule['op'][] = parent && isChoice(parent.inputType) ? ['anyOf', 'noneOf', 'filled', 'empty'] : ['filled', 'empty']
        const choices = parent ? [...parent.values].sort((a, b) => a.sortOrder - b.sortOrder) : []
        const needsChoices = rule.op === 'anyOf' || rule.op === 'noneOf'
        return (
          <div key={i} className="grid gap-2 rounded-md border p-3">
            <div className="flex flex-wrap items-center gap-2">
              <Select
                value={rule.option_id || undefined}
                disabled={disabled}
                onValueChange={(id) => {
                  const next = earlier.find((o) => o.id === id)
                  const choiceParent = !!next && isChoice(next.inputType)
                  update(i, { option_id: id, op: choiceParent ? 'anyOf' : 'filled', value_ids: [] })
                }}
              >
                <SelectTrigger size="sm" className="min-w-40 flex-1" aria-label={`Condition ${i + 1} option`}>
                  <SelectValue placeholder="Pick an option…" />
                </SelectTrigger>
                <SelectContent>
                  {earlier.map((o) => <SelectItem key={o.id} value={o.id}>{title(o)}</SelectItem>)}
                </SelectContent>
              </Select>
              <Select value={rule.op} onValueChange={(v) => update(i, { op: v as VisibilityRule['op'], value_ids: [] })} disabled={disabled || !parent}>
                <SelectTrigger size="sm" className="w-36" aria-label={`Condition ${i + 1} test`}><SelectValue /></SelectTrigger>
                <SelectContent>
                  {ops.map((o) => <SelectItem key={o} value={o}>{OP_LABEL[o]}</SelectItem>)}
                </SelectContent>
              </Select>
              <Button
                type="button" variant="ghost" size="icon" className="size-8 text-muted-foreground hover:text-destructive" disabled={disabled}
                aria-label={`Remove condition ${i + 1}`} onClick={() => onChange({ rules: rules.filter((_, j) => j !== i) })}
              >
                <Trash2Icon className="size-3.5" />
              </Button>
            </div>
            {parent && needsChoices ? (
              choices.length === 0 ? (
                <p className="text-xs text-muted-foreground">{title(parent)} has no choices yet.</p>
              ) : (
                <div className="flex flex-wrap gap-x-4 gap-y-2">
                  {choices.map((v) => (
                    <label key={v.id} className="flex items-center gap-1.5 text-sm">
                      <Checkbox
                        checked={rule.value_ids?.includes(v.id) ?? false}
                        disabled={disabled}
                        onCheckedChange={(on) => {
                          const cur = rule.value_ids ?? []
                          update(i, { value_ids: on === true ? [...cur, v.id] : cur.filter((id) => id !== v.id) })
                        }}
                      />
                      {valueContentFor(v, locale) || 'Untitled choice'}
                    </label>
                  ))}
                </div>
              )
            ) : null}
          </div>
        )
      })}

      {error ? <p className="text-xs text-destructive">{error}</p> : null}
      {earlier.length === 0 ? null : (
        <div>
          <Button type="button" variant="outline" size="sm" disabled={disabled} onClick={() => onChange({ rules: [...rules, { option_id: '', op: 'filled' }] })}>
            <PlusIcon className="size-4" />
            Add a condition
          </Button>
        </div>
      )}
    </div>
  )
}
