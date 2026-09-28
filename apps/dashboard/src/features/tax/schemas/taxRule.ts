import { z } from 'zod'
import { wholeNumber } from '@/shared/schemas/fields'
import type { SectionDiffEntry } from '@/shared/saving/saveRegistry'
import { added, changed, display, fieldDiff, removed } from '@/shared/saving/diff'

// Tax rule form - THE field list (docs/conventions.md, "Record forms")

type StoredRow = { type: string; params: unknown }

export type ExistingTaxRule = {
  id: string
  channelId: string
  name: string
  priority: number
  exclusivityGroup: string
  sortOrder: number
  isActive: boolean
  conditions: StoredRow[]
  effects: StoredRow[]
}

// Edited by SchemaForm
const ruleRowSchema = z
  .object({
    type: z.string().min(1, 'Pick a type').describe('Registered condition or effect type'),
    params: z.unknown().describe("The type's parameters, shaped by its JSON schema"),
  })
  .describe('One condition or effect of a tax rule')

export const taxRuleSchema = z
  .object({
    name: z.string().trim().min(1, 'Name is required').describe('Display name of the rule'),
    priority: wholeNumber().describe('Evaluation order - lower is evaluated earlier'),
    exclusivityGroup: z
      .string()
      .describe('At most one rule per group applies, the most specific by priority - empty string always stacks'),
    sortOrder: wholeNumber().describe('Order in rule lists - lower first'),
    isActive: z.boolean().describe('Whether the rule applies'),
    conditions: z.array(ruleRowSchema).describe('Conditions that decide when the rule applies, evaluated in order'),
    effects: z.array(ruleRowSchema).describe('Effects applied when the rule matches, such as a rate, in order'),
  })
  .describe('A tax rule: conditions that pick where it applies and effects that set the tax')

export type TaxRuleValues = z.infer<typeof taxRuleSchema>
export type RuleRow = TaxRuleValues['conditions'][number]

export const TAX_RULE_LABELS: Record<keyof TaxRuleValues, string> = {
  name: 'Name',
  priority: 'Priority',
  exclusivityGroup: 'Exclusivity group',
  sortOrder: 'Sort order',
  isActive: 'Active',
  conditions: 'Conditions',
  effects: 'Effects',
}

const toRows = (rows?: StoredRow[]): RuleRow[] => (rows ?? []).map((r) => ({ type: r.type, params: r.params ?? {} }))

export function toTaxRuleValues(rule?: ExistingTaxRule): TaxRuleValues {
  return {
    name: rule?.name ?? '',
    priority: rule?.priority ?? 0,
    exclusivityGroup: rule?.exclusivityGroup ?? '',
    sortOrder: rule?.sortOrder ?? 0,
    isActive: rule?.isActive ?? true,
    conditions: toRows(rule?.conditions),
    effects: toRows(rule?.effects),
  }
}

// Row order is the evaluation order, so it's sent as sortOrder
const toRowInput = (rows: RuleRow[]) => rows.map((r, i) => ({ type: r.type, params: r.params ?? {}, sortOrder: i }))

export function toTaxRuleInput(v: TaxRuleValues) {
  return {
    name: v.name.trim(),
    priority: v.priority,
    exclusivityGroup: v.exclusivityGroup,
    sortOrder: v.sortOrder,
    isActive: v.isActive,
    conditions: toRowInput(v.conditions),
    effects: toRowInput(v.effects),
  }
}

// "country: US, DE; rate: 7.25" - readable in the dialog without dumping JSON
export function paramsSummary(params: unknown): string {
  if (!params || typeof params !== 'object' || Array.isArray(params)) return display(params)
  return Object.entries(params as Record<string, unknown>)
    .map(([k, v]) => `${k}: ${display(v) || '—'}`)
    .join('; ')
}

function rowSummary(row: RuleRow): string {
  const params = paramsSummary(row.params)
  return params ? `${row.type} (${params})` : row.type
}

// One entry per row position - so a changed rate reads "Effects · 1: rate (rate: 7) → rate (rate: 7.25)"
// instead of the old type-list comparison, which never noticed a params edit at all
function rowsDiff(label: string, before: RuleRow[], after: RuleRow[]): SectionDiffEntry[] {
  const out: SectionDiffEntry[] = []
  for (let i = 0; i < Math.max(before.length, after.length); i++) {
    const was = before[i]
    const now = after[i]
    const name = `${label} · ${i + 1}`
    if (!was && now) out.push(added(name, rowSummary(now)))
    else if (was && !now) out.push(removed(name, rowSummary(was)))
    else if (was && now) {
      const entry = changed(name, rowSummary(was), rowSummary(now))
      if (entry) out.push(entry)
    }
  }
  return out
}

const SCALARS = ['name', 'priority', 'exclusivityGroup', 'sortOrder', 'isActive'] as const

export function taxRuleDiff(original: TaxRuleValues, draft: TaxRuleValues): SectionDiffEntry[] {
  return [
    ...fieldDiff(
      original,
      draft,
      SCALARS.map((k) => ({ label: TAX_RULE_LABELS[k], value: (o: TaxRuleValues) => o[k] })),
    ),
    ...rowsDiff('Conditions', original.conditions, draft.conditions),
    ...rowsDiff('Effects', original.effects, draft.effects),
  ]
}
