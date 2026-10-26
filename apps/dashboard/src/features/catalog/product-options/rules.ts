import type { ProductOptionPriceType, ProductOptionRuleInput } from '@/graphql/schema.types'
import { added, removed } from '@/shared/saving/diff'
import type { SectionDiffEntry } from '@/shared/saving/saveRegistry'
import type { ProductOptionRuleFieldsFragment } from '../Catalog.gql'
import {
  conditionReferences,
  isTemp,
  resolveCondition,
  optionContentFor,
  parseVisibleIfGroup,
  type ChannelPrice,
  type ProductOption,
  type VisibilityGroup,
} from '../productOptionsStaging'
import { conditionText, priceLabel } from './option-draft'

// A product's option rules as the dashboard stages them - the condition reads any option, the actions say what changes
// when it matches. A rule not saved yet carries a temp id, which the save drops so the server adds it

export type RuleAction =
  | { type: 'REQUIRE' | 'OPTIONAL'; optionId: string }
  | { type: 'SET_QUANTITY'; quantity: number }
  | { type: 'ADJUST_PRICE'; priceType: ProductOptionPriceType; priceValue: number; channelPrices: ChannelPrice[] }

export type OptionRule = {
  id: string
  labels: Record<string, string>
  condition: VisibilityGroup
  actions: RuleAction[]
}

export function rulesFromQuery(rows: readonly ProductOptionRuleFieldsFragment[]): OptionRule[] {
  return rows.map((r) => ({
    id: r.id,
    labels: Object.fromEntries(r.labels.map((l) => [l.locale, l.label])),
    condition: parseVisibleIfGroup(r.condition) ?? { rules: [] },
    actions: r.actions.flatMap((a): RuleAction[] => {
      switch (a.type) {
        case 'REQUIRE':
        case 'OPTIONAL':
          return a.optionId ? [{ type: a.type, optionId: a.optionId }] : []
        case 'SET_QUANTITY':
          return a.quantity != null ? [{ type: 'SET_QUANTITY', quantity: a.quantity }] : []
        case 'ADJUST_PRICE':
          return [{ type: 'ADJUST_PRICE', priceType: a.priceType ?? 'FIXED', priceValue: a.priceValue ?? 0, channelPrices: a.channelPrices }]
      }
    }),
  }))
}

export function ruleToInput(rule: OptionRule): ProductOptionRuleInput {
  return {
    ...(isTemp(rule.id) ? {} : { id: rule.id }),
    labels: Object.entries(rule.labels).filter(([, l]) => l.trim() !== '').map(([locale, label]) => ({ locale, label })),
    condition: rule.condition,
    actions: rule.actions.map((a) => {
      switch (a.type) {
        case 'REQUIRE':
        case 'OPTIONAL':
          return { type: a.type, optionId: a.optionId }
        case 'SET_QUANTITY':
          return { type: a.type, quantity: a.quantity }
        case 'ADJUST_PRICE':
          return { type: a.type, priceType: a.priceType, priceValue: a.priceValue, channelPrices: a.channelPrices }
      }
    }),
  }
}

// After a save that failed partway - options it added have real ids now, and the draft has to name those
export function resolveRuleIds(rules: readonly OptionRule[], resolve: (id: string) => string): OptionRule[] {
  return rules.map((r) => ({
    ...r,
    condition: resolveCondition(r.condition, resolve),
    actions: r.actions.map((a) => (a.type === 'REQUIRE' || a.type === 'OPTIONAL' ? { ...a, optionId: resolve(a.optionId) } : a)),
  }))
}

export function ruleLabel(rule: OptionRule, locale: string, defaultLocale: string): string {
  return rule.labels[locale]?.trim() || rule.labels[defaultLocale]?.trim() || ''
}

const optionName = (all: readonly ProductOption[], id: string, locale: string) => {
  const o = all.find((x) => x.id === id)
  return o ? optionContentFor(o, locale).title || 'Untitled option' : 'a removed option'
}

export function actionText(a: RuleAction, all: readonly ProductOption[], locale: string, currency: string): string {
  switch (a.type) {
    case 'REQUIRE':
      return `Require ${optionName(all, a.optionId, locale)}`
    case 'OPTIONAL':
      return `Make ${optionName(all, a.optionId, locale)} optional`
    case 'SET_QUANTITY':
      return `Set quantity to ${a.quantity}`
    case 'ADJUST_PRICE':
      return `Price ${priceLabel(a.priceValue, a.priceType, currency) ?? '+0'}`
  }
}

export function ruleSummary(rule: OptionRule, all: readonly ProductOption[], locale: string, currency: string): string {
  const when = conditionText(rule.condition, all, locale) ?? 'never'
  return `When ${when}: ${rule.actions.map((a) => actionText(a, all, locale, currency)).join(', ')}`
}

export function rulesUsing(rules: readonly OptionRule[], ref: { optionId?: string; valueId?: string }): OptionRule[] {
  return rules.filter(
    (r) =>
      conditionReferences(r.condition, ref) ||
      (ref.optionId !== undefined && r.actions.some((a) => (a.type === 'REQUIRE' || a.type === 'OPTIONAL') && a.optionId === ref.optionId)),
  )
}

// One row per rule added, removed or edited, named by what it does - a move is its own row, counted among the rules on
// both sides so an add or a delete alone doesn't read as every rule moving
export function rulesDiff(
  before: readonly OptionRule[],
  after: readonly OptionRule[],
  all: readonly ProductOption[],
  locale: string,
  currency: string,
): SectionDiffEntry[] {
  const summary = (r: OptionRule) => ruleSummary(r, all, locale, currency)
  const name = (r: OptionRule) => ruleLabel(r, locale, locale) || `When ${conditionText(r.condition, all, locale) ?? 'never'}`
  const prev = new Map(before.map((r, i) => [r.id, { rule: r, index: i }]))
  const next = new Set(after.map((r) => r.id))
  const keptBefore = before.filter((r) => next.has(r.id)).map((r) => r.id)
  const keptAfter = after.filter((r) => prev.has(r.id)).map((r) => r.id)
  const rows: SectionDiffEntry[] = []
  after.forEach((r, i) => {
    const old = prev.get(r.id)
    if (!old) {
      rows.push(added(`Rule ${i + 1}`, summary(r)))
      return
    }
    prev.delete(r.id)
    if (JSON.stringify(ruleToInput(old.rule)) !== JSON.stringify(ruleToInput(r))) {
      rows.push({ kind: 'change', label: `Rule ${i + 1}`, before: summary(old.rule), after: summary(r) })
    }
    if (keptBefore.indexOf(r.id) !== keptAfter.indexOf(r.id)) {
      rows.push({ kind: 'change', label: `Rule · ${name(r)} · position`, before: String(old.index + 1), after: String(i + 1) })
    }
  })
  for (const { rule } of prev.values()) rows.push(removed('Rule', summary(rule)))
  return rows
}
