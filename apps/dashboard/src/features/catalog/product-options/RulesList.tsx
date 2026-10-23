import { ArrowDownIcon, ArrowUpIcon, LayersIcon, PencilIcon, PlusIcon, Trash2Icon, WorkflowIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { isTemp, type ProductOption } from '../productOptionsStaging'
import { conditionText } from './option-draft'
import { actionText, ruleLabel, ruleToInput, type OptionRule } from './rules'

export function RulesList({
  rules,
  inherited = [],
  saved,
  options,
  locale,
  defaultLocale,
  currency,
  canManage,
  onAdd,
  onEdit,
  onDelete,
  onMove,
}: {
  rules: OptionRule[]
  // A product's template rules - shown first, read-only, edited on the template
  inherited?: { rule: OptionRule; template: string }[]
  // The rules as saved, so a staged edit can say so
  saved: OptionRule[]
  options: ProductOption[]
  locale: string
  defaultLocale: string
  currency: string
  canManage: boolean
  onAdd: () => void
  onEdit: (id: string) => void
  onDelete: (id: string) => void
  onMove: (from: number, to: number) => void
}) {
  const savedById = new Map(saved.map((r) => [r.id, JSON.stringify(ruleToInput(r))]))
  const arrow = (i: number, dir: 'up' | 'down') => {
    const to = dir === 'up' ? i - 1 : i + 1
    return (
      <Button
        type="button" variant="ghost" size="icon" className="size-7" disabled={to < 0 || to >= rules.length}
        aria-label={`Move rule ${i + 1} ${dir}`} onClick={() => onMove(i, to)}
      >
        {dir === 'up' ? <ArrowUpIcon className="size-3.5" /> : <ArrowDownIcon className="size-3.5" />}
      </Button>
    )
  }
  return (
    <section className="grid gap-3" aria-label="Rules">
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div className="grid gap-0.5">
          <h4 className="text-sm font-semibold">Rules</h4>
          <p className="text-xs text-muted-foreground">
            When customers answer a certain way, change what's required, fix the quantity or adjust the price.
          </p>
        </div>
        {canManage ? (
          <span title={options.length === 0 ? 'Add an option first - a rule reacts to answers' : undefined}>
            <Button type="button" size="sm" variant="outline" disabled={options.length === 0} onClick={onAdd}>
              <PlusIcon className="size-4" />
              Add rule
            </Button>
          </span>
        ) : null}
      </div>
      {inherited.length > 0 ? (
        <ul className="divide-y rounded-md border border-dashed" aria-label="Rules from templates">
          {inherited.map(({ rule, template }) => {
            const name = ruleLabel(rule, locale, defaultLocale)
            return (
              <li key={rule.id} className="flex items-start gap-3 px-4 py-3">
                <WorkflowIcon className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
                <div className="grid min-w-0 flex-1 gap-1.5">
                  <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
                    <span className="text-sm font-medium">When {conditionText(rule.condition, options, locale) ?? 'never'}</span>
                    <span className="inline-flex items-center gap-1 text-xs text-muted-foreground">
                      <LayersIcon className="size-3" />
                      from {template}
                    </span>
                  </div>
                  <ul className="flex flex-wrap gap-1.5">
                    {rule.actions.map((a, n) => (
                      <li key={n} className="rounded-md border px-2 py-0.5 text-xs">{actionText(a, options, locale, currency)}</li>
                    ))}
                    {name ? <li className="px-1 text-xs text-muted-foreground">shown as "{name}"</li> : null}
                  </ul>
                </div>
              </li>
            )
          })}
        </ul>
      ) : null}
      {rules.length === 0 ? (
        <p className="rounded-md border border-dashed px-4 py-4 text-center text-xs text-muted-foreground">
          {inherited.length > 0 ? 'No rules of its own yet.' : 'No rules yet.'}
        </p>
      ) : (
        <ul className="divide-y rounded-md border">
          {rules.map((r, i) => {
            const name = ruleLabel(r, locale, defaultLocale)
            const changed = !isTemp(r.id) && savedById.get(r.id) !== JSON.stringify(ruleToInput(r))
            return (
              <li key={r.id} className="flex items-start gap-2 px-3 py-3 sm:gap-3 sm:px-4">
                {canManage && rules.length > 1 ? <div className="flex flex-col">{arrow(i, 'up')}{arrow(i, 'down')}</div> : null}
                <WorkflowIcon className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
                <div className="grid min-w-0 flex-1 gap-1.5">
                  <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
                    {canManage ? (
                      <button type="button" className="text-left text-sm font-medium hover:underline" onClick={() => onEdit(r.id)}>
                        When {conditionText(r.condition, options, locale) ?? 'never'}
                      </button>
                    ) : (
                      <span className="text-sm font-medium">When {conditionText(r.condition, options, locale) ?? 'never'}</span>
                    )}
                    {isTemp(r.id) ? (
                      <span className="text-xs text-success">New</span>
                    ) : changed ? (
                      <span className="inline-flex items-center gap-1 text-xs text-muted-foreground">
                        <span aria-hidden className="size-1.5 rounded-full bg-warning" />
                        Unsaved changes
                      </span>
                    ) : null}
                  </div>
                  <ul className="flex flex-wrap gap-1.5" aria-label={`Rule ${i + 1} actions`}>
                    {r.actions.map((a, n) => (
                      <li key={n} className="rounded-md border px-2 py-0.5 text-xs">{actionText(a, options, locale, currency)}</li>
                    ))}
                    {name ? <li className="px-1 text-xs text-muted-foreground">shown as "{name}"</li> : null}
                  </ul>
                </div>
                {canManage ? (
                  <div className="flex shrink-0 items-center gap-1">
                    <Button type="button" variant="ghost" size="icon" className="size-8" aria-label={`Edit rule ${i + 1}`} onClick={() => onEdit(r.id)}>
                      <PencilIcon className="size-3.5" />
                    </Button>
                    <Button
                      type="button" variant="ghost" size="icon" className="size-8 text-muted-foreground hover:text-destructive"
                      aria-label={`Delete rule ${i + 1}`} onClick={() => onDelete(r.id)}
                    >
                      <Trash2Icon className="size-3.5" />
                    </Button>
                  </div>
                ) : null}
              </li>
            )
          })}
        </ul>
      )}
    </section>
  )
}
