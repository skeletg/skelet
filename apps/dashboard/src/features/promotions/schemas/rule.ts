import { z } from 'zod'
import { dateRangeIssues, isoToLocalInput, localInputToISO, numberText, wholeNumber } from '@/shared/schemas/fields'
import type { SectionDiffEntry } from '@/shared/saving/saveRegistry'
import { changed, fieldDiff } from '@/shared/saving/diff'

// Promotion rule - THE field list (docs/conventions.md, "Record forms"): identity, lifecycle,
// creative, stacking, the condition and action rows, validity and limits, and the header locale's
// storefront copy. A row's `params` are shaped by its type's schema from the backend (SchemaForm), so
// they're opaque here and validated by the service. Codes are rows with their own actions

export type StopMode = 'NONE' | 'SAME_ACTION_TYPE' | 'ALL'

const subrow = z.object({
  type: z.string().min(1, 'Pick a type, or remove the row'),
  params: z.unknown(),
  paramsVersion: z.number().optional(),
  sortOrder: z.number().optional(),
})

export type RuleRow = z.infer<typeof subrow>

// Blank means "no limit"
const optionalCount = numberText({ optional: true, int: true, min: 1 })
const HEX = /^#[0-9a-f]{6}$/i

export const ruleSchema = z
  .object({
    name: z.string().trim().min(1, 'Name is required'),
    slug: z.string().trim().min(1, 'Slug is required'),
    description: z.string(),
    publicLabel: z.string(),
    requiresCode: z.boolean(),
    isActive: z.boolean(),
    priority: wholeNumber(),
    stopAfterApply: z.enum(['NONE', 'SAME_ACTION_TYPE', 'ALL']),
    exclusivityGroup: z.string(),
    // datetime-local text, local time - converted to ISO on the way out
    validFrom: z.string(),
    validTo: z.string(),
    maxUses: optionalCount,
    perCustomerLimit: optionalCount,
    // '' = none - the backend clears the reference on an empty string
    labelMediaId: z.string(),
    bannerMediaId: z.string(),
    pillText: z.string().max(40, 'Keep it short - 40 characters at most'),
    pillTextColor: z.string().regex(HEX, 'A #RRGGBB colour'),
    pillBgColor: z.string().regex(HEX, 'A #RRGGBB colour'),
    conditions: z.array(subrow),
    actions: z.array(subrow),
    // The header locale's storefront copy - edit only
    contentLabel: z.string(),
    contentTerms: z.string(),
  })
  .superRefine((v, ctx) => {
    dateRangeIssues(v.validFrom, v.validTo, ['validTo'], ctx)
    // A translation row needs its label - terms alone would be silently dropped
    if (!v.contentLabel.trim() && v.contentTerms.trim()) {
      ctx.addIssue({ code: 'custom', path: ['contentLabel'], message: 'Terms need a label to go with them' })
    }
  })

export type RuleValues = z.infer<typeof ruleSchema>

export const RULE_LABELS: Record<keyof RuleValues, string> = {
  name: 'Name',
  slug: 'Slug',
  description: 'Description',
  publicLabel: 'Public label',
  requiresCode: 'Requires a code',
  isActive: 'Live',
  priority: 'Priority',
  stopAfterApply: 'When it fires',
  exclusivityGroup: 'Exclusivity group',
  validFrom: 'Valid from',
  validTo: 'Valid to',
  maxUses: 'Max uses',
  perCustomerLimit: 'Per-customer limit',
  labelMediaId: 'Label image',
  bannerMediaId: 'Banner image',
  pillText: 'Pill text',
  pillTextColor: 'Pill text colour',
  pillBgColor: 'Pill background',
  conditions: 'Conditions',
  actions: 'Actions',
  contentLabel: 'Storefront label',
  contentTerms: 'Terms',
}

type ExistingRule = {
  slug: string
  name: string
  description: string
  publicLabel: string
  requiresCode: boolean
  priority: number
  stopAfterApply: StopMode
  exclusivityGroup: string
  validFrom: string | null
  validTo: string | null
  maxUses: number | null
  perCustomerLimit: number | null
  isActive: boolean
  labelMediaId: string | null
  bannerMediaId: string | null
  pillText: string
  pillTextColor: string
  pillBgColor: string
  contents: { locale: string; label: string; terms: string }[]
  conditions: { type: string; params: unknown; paramsVersion: number; sortOrder: number }[]
  actions: { type: string; params: unknown; paramsVersion: number; sortOrder: number }[]
}

const toRows = (rows: ExistingRule['conditions']): RuleRow[] =>
  rows.map((r) => ({ type: r.type, params: r.params ?? {}, paramsVersion: r.paramsVersion, sortOrder: r.sortOrder }))

export function toRuleValues(rule: ExistingRule | undefined, locale: string): RuleValues {
  const content = rule?.contents.find((c) => c.locale === locale)
  return {
    name: rule?.name ?? '',
    slug: rule?.slug ?? '',
    description: rule?.description ?? '',
    publicLabel: rule?.publicLabel ?? '',
    requiresCode: rule?.requiresCode ?? true,
    isActive: rule?.isActive ?? true,
    priority: rule?.priority ?? 0,
    stopAfterApply: rule?.stopAfterApply ?? 'NONE',
    exclusivityGroup: rule?.exclusivityGroup ?? '',
    validFrom: isoToLocalInput(rule?.validFrom ?? null),
    validTo: isoToLocalInput(rule?.validTo ?? null),
    maxUses: rule?.maxUses?.toString() ?? '',
    perCustomerLimit: rule?.perCustomerLimit?.toString() ?? '',
    labelMediaId: rule?.labelMediaId ?? '',
    bannerMediaId: rule?.bannerMediaId ?? '',
    pillText: rule?.pillText ?? '',
    // Colour inputs need a valid #RRGGBB to draw a swatch - a sensible red/white pair until chosen
    pillTextColor: rule?.pillTextColor || '#ffffff',
    pillBgColor: rule?.pillBgColor || '#dc2626',
    conditions: toRows(rule?.conditions ?? []),
    actions: toRows(rule?.actions ?? []),
    contentLabel: content?.label ?? '',
    contentTerms: content?.terms ?? '',
  }
}

const toRowInputs = (rows: RuleRow[]) => rows.map((r, i) => ({ type: r.type, params: r.params ?? {}, sortOrder: r.sortOrder ?? i }))

// The rule itself - create and update take the same fields
export function toRuleInput(v: RuleValues) {
  const pill = v.pillText.trim()
  return {
    slug: v.slug.trim(),
    name: v.name.trim(),
    description: v.description,
    publicLabel: v.publicLabel,
    requiresCode: v.requiresCode,
    priority: v.priority,
    stopAfterApply: v.stopAfterApply,
    exclusivityGroup: v.exclusivityGroup,
    validFrom: localInputToISO(v.validFrom),
    validTo: localInputToISO(v.validTo),
    maxUses: v.maxUses.trim() ? Number(v.maxUses) : null,
    perCustomerLimit: v.perCustomerLimit.trim() ? Number(v.perCustomerLimit) : null,
    isActive: v.isActive,
    labelMediaId: v.labelMediaId,
    bannerMediaId: v.bannerMediaId,
    // Empty text = no pill, and its colours then mean "the channel's default palette"
    pillText: pill,
    pillTextColor: pill ? v.pillTextColor : '',
    pillBgColor: pill ? v.pillBgColor : '',
    conditions: toRowInputs(v.conditions),
    actions: toRowInputs(v.actions),
  }
}

// The storefront copy for the header locale - written only when it changed and has a label; clearing
// the label is a removal, which the Remove translation action does
export function contentChange(before: RuleValues, after: RuleValues) {
  const label = after.contentLabel.trim()
  const changedCopy = label !== before.contentLabel.trim() || after.contentTerms.trim() !== before.contentTerms.trim()
  return changedCopy && label ? { label, terms: after.contentTerms.trim() || null } : null
}

const RULE_FIELDS: (keyof RuleValues)[] = [
  'name',
  'slug',
  'description',
  'publicLabel',
  'requiresCode',
  'isActive',
  'priority',
  'stopAfterApply',
  'exclusivityGroup',
  'validFrom',
  'validTo',
  'maxUses',
  'perCustomerLimit',
  'labelMediaId',
  'bannerMediaId',
  'pillText',
  'pillTextColor',
  'pillBgColor',
  'contentLabel',
  'contentTerms',
]

// Rows read as their types in order - a params-only edit still shows, as the types plus "(edited)"
function rowsEntry(label: string, before: RuleRow[], after: RuleRow[]): SectionDiffEntry | null {
  if (JSON.stringify(before) === JSON.stringify(after)) return null
  const types = (rows: RuleRow[]) => rows.map((r) => r.type || '(new)').join(', ') || 'none'
  const b = types(before)
  const a = types(after)
  return changed(label, b, a === b ? `${a} (edited)` : a)
}

export function ruleDiff(original: RuleValues, draft: RuleValues): SectionDiffEntry[] {
  return [
    ...fieldDiff(
      original,
      draft,
      RULE_FIELDS.map((k) => ({ label: RULE_LABELS[k], value: (v: RuleValues) => v[k] })),
    ),
    ...[rowsEntry(RULE_LABELS.conditions, original.conditions, draft.conditions), rowsEntry(RULE_LABELS.actions, original.actions, draft.actions)].filter(
      (e) => e !== null,
    ),
  ]
}
