import { useState } from 'react'
import { ChevronDownIcon, LanguagesIcon, PlusIcon, Trash2Icon } from 'lucide-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import type { ProductOptionPriceType } from '@/graphql/schema.types'
import { toMinor } from '@/lib/format'
import { ChannelSurchargeInputs } from '../ChannelSurchargeInputs'
import { invalidChannelPrices, rawChannelPrices } from '../channelSurcharges'
import { newTempId, optionContentFor, type ProductOption, type VisibilityRule } from '../productOptionsStaging'
import { PriceInput } from './ChoicesEditor'
import { ConditionEditor } from './ConditionEditor'
import { conditionFrom, parsePrice, priceText, type DraftContext } from './option-draft'
import type { OptionRule, RuleAction } from './rules'

type ActionType = RuleAction['type']

const ACTION_LABEL: Record<ActionType, string> = {
  REQUIRE: 'Require an option',
  OPTIONAL: 'Make an option optional',
  SET_QUANTITY: 'Set the cart quantity',
  ADJUST_PRICE: 'Change the price',
}

type ActionDraft = {
  key: string
  type: ActionType
  optionId: string
  quantity: string
  price: string
  priceType: ProductOptionPriceType
  channelPrices: Record<string, string>
}

const blankAction = (type: ActionType = 'REQUIRE'): ActionDraft => ({
  key: newTempId(), type, optionId: '', quantity: '', price: '', priceType: 'FIXED', channelPrices: {},
})

function actionDraft(a: RuleAction, ctx: DraftContext): ActionDraft {
  const base = blankAction(a.type)
  switch (a.type) {
    case 'REQUIRE':
    case 'OPTIONAL':
      return { ...base, optionId: a.optionId }
    case 'SET_QUANTITY':
      return { ...base, quantity: String(a.quantity) }
    case 'ADJUST_PRICE':
      return {
        ...base, priceType: a.priceType, price: priceText(a.priceValue, a.priceType, ctx.baseCurrency),
        channelPrices: rawChannelPrices(a.channelPrices, ctx.channels),
      }
  }
}

type ActionErrors = Record<string, string>

function actionError(a: ActionDraft, ctx: DraftContext): string | null {
  switch (a.type) {
    case 'REQUIRE':
    case 'OPTIONAL':
      return a.optionId ? null : 'Pick an option.'
    case 'SET_QUANTITY':
      return Number.isInteger(Number(a.quantity)) && Number(a.quantity) >= 1 ? null : 'A whole number, 1 or more.'
    case 'ADJUST_PRICE':
      if (a.price.trim() === '') return 'Enter an amount - a minus makes it a discount.'
      if (parsePrice(a.price, a.priceType, ctx.baseCurrency) === null) {
        return a.priceType === 'PERCENT' ? 'Use a whole percentage, like 10.' : `Enter an amount in ${ctx.baseCurrency}, like -2.50.`
      }
      return a.priceType === 'FIXED' && invalidChannelPrices(ctx.channels, a.channelPrices).length > 0 ? 'Fix the store amounts.' : null
  }
}

function toAction(a: ActionDraft, ctx: DraftContext): RuleAction {
  switch (a.type) {
    case 'REQUIRE':
    case 'OPTIONAL':
      return { type: a.type, optionId: a.optionId }
    case 'SET_QUANTITY':
      return { type: 'SET_QUANTITY', quantity: Number(a.quantity) }
    case 'ADJUST_PRICE':
      return {
        type: 'ADJUST_PRICE', priceType: a.priceType, priceValue: parsePrice(a.price, a.priceType, ctx.baseCurrency) ?? 0,
        channelPrices: a.priceType === 'FIXED'
          ? ctx.channels.flatMap((ch) => {
              const minor = toMinor(a.channelPrices[ch.id] ?? '', ch.currencyCode)
              return minor === null ? [] : [{ channelId: ch.id, priceValue: minor }]
            })
          : [],
      }
  }
}

type RuleDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  // The rule being edited, or null to add one
  rule: OptionRule | null
  options: ProductOption[]
  locale: string
  defaultLocale: string
  multilingual: boolean
  ctx: DraftContext
  onDone: (rule: OptionRule) => void
}

export function RuleDialog(props: RuleDialogProps) {
  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className="flex max-h-[92vh] flex-col gap-0 p-0 sm:max-w-[min(94vw,68rem)]">
        {props.open ? <RuleDialogBody key={`${props.rule?.id ?? 'new'}:${props.locale}`} {...props} /> : null}
      </DialogContent>
    </Dialog>
  )
}

function RuleDialogBody({ onOpenChange, rule, options, locale, defaultLocale, multilingual, ctx, onDone }: RuleDialogProps) {
  // Another language only renames the rule - what it reacts to and what it does belong to every language
  const translating = !!rule && locale !== defaultLocale
  const [label, setLabel] = useState(rule?.labels[locale] ?? '')
  const [logic, setLogic] = useState<'and' | 'or'>(rule?.condition.logic === 'or' ? 'or' : 'and')
  const [conditions, setConditions] = useState<VisibilityRule[]>(rule?.condition.rules ?? [])
  const [actions, setActions] = useState<ActionDraft[]>(() => rule?.actions.map((a) => actionDraft(a, ctx)) ?? [blankAction()])
  const [attempted, setAttempted] = useState(false)
  const nested = (rule?.condition.groups?.length ?? 0) > 0

  const actionErrors: ActionErrors = {}
  for (const a of actions) {
    const e = actionError(a, ctx)
    if (e) actionErrors[a.key] = e
  }
  const conditionError =
    !nested && conditions.length === 0
      ? 'Add a condition - a rule needs an answer to react to.'
      : conditions.some((r) => !r.option_id || ((r.op === 'anyOf' || r.op === 'noneOf') && (r.value_ids?.length ?? 0) === 0))
        ? 'Finish or remove the incomplete condition.'
        : null
  const actionsError =
    actions.length === 0
      ? 'Add at least one action.'
      : actions.filter((a) => a.type === 'SET_QUANTITY').length > 1
        ? 'A rule can set the quantity once.'
        : null
  const valid = !conditionError && !actionsError && Object.keys(actionErrors).length === 0
  const show = attempted && !valid

  const patch = (key: string, p: Partial<ActionDraft>) => setActions((list) => list.map((a) => (a.key === key ? { ...a, ...p } : a)))

  function done() {
    if (!valid) {
      setAttempted(true)
      return
    }
    onDone({
      id: rule?.id ?? newTempId(),
      labels: { ...(rule?.labels ?? {}), [locale]: label.trim() },
      condition: nested && rule ? rule.condition : conditionFrom({ logic, rules: conditions }) ?? { rules: [] },
      actions: actions.map((a) => toAction(a, ctx)),
    })
    onOpenChange(false)
  }

  const title = (o: ProductOption) => optionContentFor(o, locale).title || 'Untitled option'

  return (
    <>
      <DialogHeader className="border-b px-6 py-4">
        <DialogTitle>{rule ? 'Edit rule' : 'New rule'}</DialogTitle>
        <DialogDescription>When customers answer a certain way, change what's required, fix the quantity or adjust the price.</DialogDescription>
      </DialogHeader>

      <div className="min-h-0 flex-1 overflow-y-auto px-6 py-5">
        <div className="grid gap-6">
          {translating ? (
            <Alert>
              <LanguagesIcon className="size-4" />
              <AlertDescription>
                Translating to {locale}. Only the name changes here - switch the language back to {defaultLocale} to change what the rule does.
              </AlertDescription>
            </Alert>
          ) : null}
          <div className="grid gap-8 lg:grid-cols-2 lg:gap-10">
            <section className="grid content-start gap-3">
              <div className="grid gap-0.5">
                <h3 className="text-sm font-semibold">When</h3>
                <p className="text-xs text-muted-foreground">The answers this rule reacts to. It can read any option.</p>
              </div>
              <ConditionEditor
                earlier={options}
                isNew={false}
                forRule
                locale={locale}
                logic={logic}
                rules={conditions}
                nested={nested}
                disabled={translating}
                error={show && conditionError ? conditionError : undefined}
                onChange={(p) => {
                  if (p.logic) setLogic(p.logic)
                  if (p.rules) setConditions(p.rules)
                }}
              />
            </section>

            <section className="grid content-start gap-3">
              <div className="grid gap-0.5">
                <h3 className="text-sm font-semibold">Then</h3>
                <p className="text-xs text-muted-foreground">What changes while the condition holds. Hidden options are never required.</p>
              </div>
              <ul className="grid gap-2">
                {actions.map((a, n) => (
                  <li key={a.key} className="grid gap-2 rounded-md border p-3">
                    <div className="flex flex-wrap items-start gap-2">
                      <Select
                        value={a.type}
                        disabled={translating}
                        onValueChange={(t) => patch(a.key, { ...blankAction(t as ActionType), key: a.key })}
                      >
                        <SelectTrigger size="sm" className="w-52" aria-label={`Action ${n + 1} type`}>
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          {(Object.keys(ACTION_LABEL) as ActionType[]).map((t) => (
                            <SelectItem key={t} value={t}>{ACTION_LABEL[t]}</SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                      <div className="min-w-0 flex-1">
                        {a.type === 'REQUIRE' || a.type === 'OPTIONAL' ? (
                          <Select value={a.optionId || undefined} disabled={translating} onValueChange={(id) => patch(a.key, { optionId: id })}>
                            <SelectTrigger size="sm" className="w-full" aria-label={`Action ${n + 1} option`}>
                              <SelectValue placeholder="Pick an option…" />
                            </SelectTrigger>
                            <SelectContent>
                              {options.map((o) => <SelectItem key={o.id} value={o.id}>{title(o)}</SelectItem>)}
                            </SelectContent>
                          </Select>
                        ) : a.type === 'SET_QUANTITY' ? (
                          <Input
                            inputMode="numeric" placeholder="e.g. 2" className="h-8 w-32" value={a.quantity} disabled={translating}
                            aria-label={`Action ${n + 1} quantity`} onChange={(e) => patch(a.key, { quantity: e.target.value })}
                          />
                        ) : (
                          <div className="w-48">
                            <PriceInput
                              label={`Action ${n + 1} amount`} value={a.price} type={a.priceType} currency={ctx.baseCurrency} disabled={translating}
                              onValue={(price) => patch(a.key, { price })} onType={(priceType) => patch(a.key, { priceType })}
                            />
                          </div>
                        )}
                      </div>
                      <Button
                        type="button" variant="ghost" size="icon" className="size-8 text-muted-foreground hover:text-destructive" disabled={translating}
                        aria-label={`Remove action ${n + 1}`} onClick={() => setActions((list) => list.filter((x) => x.key !== a.key))}
                      >
                        <Trash2Icon className="size-3.5" />
                      </Button>
                    </div>
                    {show && actionErrors[a.key] ? <p className="text-xs text-destructive">{actionErrors[a.key]}</p> : null}
                    {a.type === 'ADJUST_PRICE' && a.priceType === 'FIXED' && ctx.channels.length > 0 ? (
                      <Collapsible defaultOpen={Object.values(a.channelPrices).some((v) => v.trim() !== '')}>
                        <CollapsibleTrigger className="group/stores flex items-center gap-1 text-xs font-medium">
                          <ChevronDownIcon className="size-3.5 transition-transform group-data-[state=closed]/stores:-rotate-90" />
                          Different amount per store
                        </CollapsibleTrigger>
                        <CollapsibleContent className="pt-2">
                          <ChannelSurchargeInputs
                            idPrefix={`${a.key}-store`} channels={ctx.channels} baseCurrency={ctx.baseCurrency}
                            baseAmount={parsePrice(a.price, 'FIXED', ctx.baseCurrency) ?? 0} values={a.channelPrices} disabled={translating}
                            onChange={(id, raw) => patch(a.key, { channelPrices: { ...a.channelPrices, [id]: raw } })}
                          />
                        </CollapsibleContent>
                      </Collapsible>
                    ) : null}
                  </li>
                ))}
              </ul>
              {show && actionsError ? <p className="text-xs text-destructive">{actionsError}</p> : null}
              <div>
                <Button type="button" variant="outline" size="sm" disabled={translating} onClick={() => setActions((list) => [...list, blankAction()])}>
                  <PlusIcon className="size-4" />
                  Add an action
                </Button>
              </div>

              <div className="mt-2 grid gap-1.5">
                <Label htmlFor="rule-label">
                  Name on the cart line {multilingual ? <span className="font-normal text-muted-foreground">({locale})</span> : null}
                </Label>
                <Input id="rule-label" value={label} placeholder="e.g. Gift set discount" onChange={(e) => setLabel(e.target.value)} />
                <p className="text-xs text-muted-foreground">Shown next to a price change. Left empty, customers see "Adjustment".</p>
              </div>
            </section>
          </div>
        </div>
      </div>

      <DialogFooter className="mx-0 mb-0 flex-row items-center justify-end gap-2 px-6 py-4">
        {show ? <p className="mr-auto text-sm text-destructive">Fix the highlighted fields first.</p> : null}
        <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>Cancel</Button>
        <Button type="button" onClick={done}>{rule ? 'Done' : 'Add rule'}</Button>
      </DialogFooter>
    </>
  )
}
