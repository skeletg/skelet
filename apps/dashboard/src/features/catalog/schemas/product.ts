import { z } from 'zod'
import type { SectionDiffEntry } from '@/shared/saving/saveRegistry'
import { added, changed, fieldDiff, removed } from '@/shared/saving/diff'
import { formatCents } from '@/lib/format'
import { richTextToPlainText, type RichTextValue } from '../LexicalRichTextEditor'

// Product page - its sections each stage their own form (shared/saving/useSectionForm), so the field
// lists live here one section at a time

// Money is typed in major units ("12.50") and stored as integer cents
export const centsToDollars = (cents: number) => (cents / 100).toFixed(2)
export const dollarsToCents = (input: string) => {
  const n = Number(input)
  return Number.isFinite(n) ? Math.round(n * 100) : 0
}
const money = z.string().refine((v) => v.trim() !== '' && Number.isFinite(Number(v)) && Number(v) >= 0, 'Zero or more')

// ---------- General: the locale's name, SKU, tax class and default price ----------

export const productGeneralSchema = z.object({
  name: z.string().trim().min(1, 'Name is required'),
  sku: z.string(),
  // Blank means the default class
  taxClass: z.string(),
  priceDollars: money,
})

export type ProductGeneralValues = z.infer<typeof productGeneralSchema>

export const PRODUCT_GENERAL_LABELS: Record<keyof ProductGeneralValues, string> = {
  name: 'Name',
  sku: 'SKU',
  taxClass: 'Tax class',
  priceDollars: 'Default price',
}

type GeneralProduct = {
  name?: string | null
  sku?: string | null
  taxClass?: string | null
  defaultPrice: number
  content: { locale: string; name?: string | null }[]
}

export function toProductGeneralValues(product: GeneralProduct, locale: string): ProductGeneralValues {
  const content = product.content.find((c) => c.locale === locale)
  return {
    name: content?.name ?? product.name ?? '',
    sku: product.sku ?? '',
    taxClass: product.taxClass ?? 'standard',
    priceDollars: centsToDollars(product.defaultPrice),
  }
}

// The section's writes, each only when its part changed: SKU and price share the product update,
// the tax class and the locale's name have their own
export function productGeneralChanges(before: ProductGeneralValues, after: ProductGeneralValues) {
  const skuOrPrice = before.sku !== after.sku || dollarsToCents(before.priceDollars) !== dollarsToCents(after.priceDollars)
  return {
    update: skuOrPrice ? { sku: after.sku || undefined, defaultPrice: dollarsToCents(after.priceDollars) } : null,
    taxClass: before.taxClass.trim() !== after.taxClass.trim() ? after.taxClass.trim() || 'standard' : null,
    name: before.name.trim() !== after.name.trim() ? after.name.trim() : null,
  }
}

export function productGeneralDiff(currency: string) {
  return (original: ProductGeneralValues, draft: ProductGeneralValues): SectionDiffEntry[] => {
    const price = changed(
      PRODUCT_GENERAL_LABELS.priceDollars,
      formatCents(dollarsToCents(original.priceDollars), currency),
      formatCents(dollarsToCents(draft.priceDollars), currency),
    )
    return [
      ...fieldDiff(original, draft, [
        { label: PRODUCT_GENERAL_LABELS.name, value: (v) => v.name },
        { label: PRODUCT_GENERAL_LABELS.sku, value: (v) => v.sku },
        { label: PRODUCT_GENERAL_LABELS.taxClass, value: (v) => v.taxClass.trim() || 'standard' },
      ]),
      ...(price ? [price] : []),
    ]
  }
}

// ---------- Advanced: cost, sale pricing, dimensions and order quantity limits ----------

// All typed text - blank means "not set"
const blankOrMoney = z.string().refine((v) => v.trim() === '' || (Number.isFinite(Number(v)) && Number(v) >= 0), 'Zero or more')
const blankOrCount = z.string().refine((v) => v.trim() === '' || (Number.isFinite(Number(v)) && Number(v) >= 0), 'Zero or more')

export const productAdvancedSchema = z
  .object({
    cost: blankOrMoney,
    compareAt: blankOrMoney,
    special: blankOrMoney,
    // datetime-local text, local time
    specialFrom: z.string(),
    specialTo: z.string(),
    weight: blankOrCount,
    length: blankOrCount,
    width: blankOrCount,
    height: blankOrCount,
    minQty: blankOrCount,
    maxQty: blankOrCount,
  })
  .superRefine((v, ctx) => {
    if (v.specialFrom && v.specialTo && new Date(v.specialTo) <= new Date(v.specialFrom)) {
      ctx.addIssue({ code: 'custom', path: ['specialTo'], message: 'Ends after it starts' })
    }
    if (v.minQty.trim() && v.maxQty.trim() && Number(v.maxQty) > 0 && Number(v.minQty) > Number(v.maxQty)) {
      ctx.addIssue({ code: 'custom', path: ['maxQty'], message: 'At least the minimum' })
    }
  })

export type ProductAdvancedValues = z.infer<typeof productAdvancedSchema>

export const PRODUCT_ADVANCED_LABELS: Record<keyof ProductAdvancedValues, string> = {
  cost: 'Unit cost',
  compareAt: 'Compare-at price',
  special: 'Special price',
  specialFrom: 'Sale starts',
  specialTo: 'Sale ends',
  weight: 'Weight',
  length: 'Length',
  width: 'Width',
  height: 'Height',
  minQty: 'Min order qty',
  maxQty: 'Max order qty',
}

type AdvancedProduct = {
  compareAtPrice?: number | null
  specialPrice?: number | null
  specialFrom?: string | null
  specialTo?: string | null
  weightGrams?: number | null
  lengthMm?: number | null
  widthMm?: number | null
  heightMm?: number | null
  minOrderQty?: number | null
  maxOrderQty?: number | null
  costCents?: number | null
}

export function isoToLocalInput(iso?: string | null): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}

const localInputToISO = (v: string) => {
  if (!v) return null
  const d = new Date(v)
  return Number.isNaN(d.getTime()) ? null : d.toISOString()
}

// Zero means "not set" for the dimension and quantity columns
const positive = (v?: number | null) => (v != null && v > 0 ? String(v) : '')
const optionalMoney = (cents?: number | null) => (cents != null ? centsToDollars(cents) : '')

export function toProductAdvancedValues(p: AdvancedProduct): ProductAdvancedValues {
  return {
    cost: optionalMoney(p.costCents),
    compareAt: optionalMoney(p.compareAtPrice),
    special: optionalMoney(p.specialPrice),
    specialFrom: isoToLocalInput(p.specialFrom),
    specialTo: isoToLocalInput(p.specialTo),
    weight: positive(p.weightGrams),
    length: positive(p.lengthMm),
    width: positive(p.widthMm),
    height: positive(p.heightMm),
    minQty: positive(p.minOrderQty),
    maxQty: positive(p.maxOrderQty),
  }
}

const differs = <K extends keyof ProductAdvancedValues>(a: ProductAdvancedValues, b: ProductAdvancedValues, keys: K[]) =>
  keys.some((k) => a[k].trim() !== b[k].trim())
const whole = (v: string) => (v.trim() === '' ? 0 : Math.max(0, Math.round(Number(v))) || 0)

// One product update carrying only the groups that changed. Compare-at and special price send the
// clear flags when emptied; the special trio travels together - a price replaces the whole schedule
export function toProductAdvancedInput(before: ProductAdvancedValues, v: ProductAdvancedValues) {
  return {
    ...(differs(before, v, ['compareAt']) ? (v.compareAt.trim() ? { compareAtPrice: dollarsToCents(v.compareAt) } : { clearCompareAtPrice: true }) : {}),
    ...(differs(before, v, ['special', 'specialFrom', 'specialTo'])
      ? v.special.trim()
        ? { specialPrice: dollarsToCents(v.special), specialFrom: localInputToISO(v.specialFrom), specialTo: localInputToISO(v.specialTo) }
        : { clearSpecialPrice: true }
      : {}),
    ...(differs(before, v, ['weight', 'length', 'width', 'height'])
      ? { weightGrams: whole(v.weight), lengthMm: whole(v.length), widthMm: whole(v.width), heightMm: whole(v.height) }
      : {}),
    ...(differs(before, v, ['minQty', 'maxQty'])
      ? { minOrderQty: v.minQty.trim() ? Math.max(0, Number(v.minQty)) || 0 : 0, maxOrderQty: v.maxQty.trim() ? Math.max(0, Number(v.maxQty)) || 0 : 0 }
      : {}),
    ...(differs(before, v, ['cost']) ? { costCents: v.cost.trim() ? dollarsToCents(v.cost) : 0 } : {}),
  }
}

const moneyText = (s: string) => (s.trim() === '' ? '' : (Number(s) || 0).toFixed(2))

export function productAdvancedDiff(original: ProductAdvancedValues, draft: ProductAdvancedValues): SectionDiffEntry[] {
  const sale = (v: ProductAdvancedValues) =>
    v.special.trim() ? `${moneyText(v.special)}${v.specialFrom ? ` from ${v.specialFrom}` : ''}${v.specialTo ? ` until ${v.specialTo}` : ''}` : ''
  const dims = (v: ProductAdvancedValues) =>
    [v.weight && `${v.weight}g`, (v.length || v.width || v.height) && `${v.length || '?'}x${v.width || '?'}x${v.height || '?'}mm`].filter(Boolean).join(' ')
  const qty = (v: ProductAdvancedValues) => (v.minQty || v.maxQty ? `${v.minQty || '-'} to ${v.maxQty || '-'}` : '')
  return fieldDiff(original, draft, [
    { label: PRODUCT_ADVANCED_LABELS.cost, value: (v) => moneyText(v.cost) },
    { label: PRODUCT_ADVANCED_LABELS.compareAt, value: (v) => moneyText(v.compareAt) },
    { label: PRODUCT_ADVANCED_LABELS.special, value: sale },
    { label: 'Weight / dimensions', value: dims },
    { label: 'Order quantity limits', value: qty },
  ])
}

// ---------- Tier pricing: quantity breaks, for everyone or one customer group ----------

const tierRow = z.object({
  // '' = all groups
  customerGroupId: z.string(),
  minQty: z.string(),
  price: z.string(),
})

export type ProductTierRow = z.infer<typeof tierRow>

const tierKey = (r: ProductTierRow) => `${r.customerGroupId}:${Number(r.minQty)}`

// The set is replaced whole on save, so a half-filled row is refused rather than dropped
export const productTiersSchema = z.object({ tiers: z.array(tierRow) }).superRefine((v, ctx) => {
  const seen = new Set<string>()
  v.tiers.forEach((r, i) => {
    const qty = Number(r.minQty)
    if (!r.minQty.trim() || !Number.isFinite(qty) || qty <= 0) {
      ctx.addIssue({ code: 'custom', path: ['tiers', i, 'minQty'], message: 'More than zero' })
    } else if (seen.has(tierKey(r))) {
      ctx.addIssue({ code: 'custom', path: ['tiers', i, 'minQty'], message: 'This group already has a tier at this quantity' })
    }
    seen.add(tierKey(r))
    if (!r.price.trim() || !Number.isFinite(Number(r.price)) || Number(r.price) < 0) {
      ctx.addIssue({ code: 'custom', path: ['tiers', i, 'price'], message: 'Zero or more' })
    }
  })
})

export type ProductTierValues = z.infer<typeof productTiersSchema>

export const PRODUCT_TIER_LABELS: Record<keyof ProductTierValues, string> = { tiers: 'Tier pricing' }

type ExistingTier = { customerGroupId?: string | null; minQty: number; price: number }

export function toProductTierValues(prices: ExistingTier[]): ProductTierValues {
  return { tiers: prices.map((p) => ({ customerGroupId: p.customerGroupId ?? '', minQty: String(p.minQty), price: centsToDollars(p.price) })) }
}

export function toProductTierInput(v: ProductTierValues) {
  return v.tiers.map((r) => ({ customerGroupId: r.customerGroupId || null, minQty: Number(r.minQty), price: dollarsToCents(r.price) }))
}

// One entry per tier added, repriced or removed, matched on group + quantity - reordering is no change
export function productTiersDiff(groupName: (id: string) => string) {
  return (original: ProductTierValues, draft: ProductTierValues): SectionDiffEntry[] => {
    const label = (r: ProductTierRow) => `Tier · ${groupName(r.customerGroupId)} · ${r.minQty.trim() ? `${Number(r.minQty)}+` : '?'}`
    const at = (r: ProductTierRow) => (r.price.trim() ? `@ ${moneyText(r.price)}` : '')
    const before = new Map(original.tiers.map((r) => [tierKey(r), r]))
    const after = new Set(draft.tiers.map(tierKey))
    const out: SectionDiffEntry[] = []
    for (const r of draft.tiers) {
      const prev = before.get(tierKey(r))
      const entry = prev ? changed(label(r), at(prev), at(r)) : added(label(r), at(r))
      if (entry) out.push(entry)
    }
    for (const o of original.tiers) if (!after.has(tierKey(o))) out.push(removed(label(o), at(o)))
    return out
  }
}

// ---------- Recommendations: the curated related, upsell and cross-sell links ----------

// Each list is the curated display order - a reorder is a change
export const productLinksSchema = z.object({
  related: z.array(z.string()),
  upsell: z.array(z.string()),
  crossSell: z.array(z.string()),
})

export type ProductLinksValues = z.infer<typeof productLinksSchema>

export const PRODUCT_LINK_LABELS: Record<keyof ProductLinksValues, string> = {
  related: 'Related links',
  upsell: 'Upsell links',
  crossSell: 'Cross-sell links',
}

export const PRODUCT_LINK_KINDS = { related: 'RELATED', upsell: 'UPSELL', crossSell: 'CROSSSELL' } as const

const sameList = (a: string[], b: string[]) => a.length === b.length && a.every((id, i) => id === b[i])

// The kinds whose list changed - each is its own replace-all write
export function changedLinkKinds(before: ProductLinksValues, after: ProductLinksValues) {
  return (Object.keys(PRODUCT_LINK_KINDS) as (keyof ProductLinksValues)[]).filter((k) => !sameList(before[k], after[k]))
}

export function productLinksDiff(name: (id: string) => string) {
  return (original: ProductLinksValues, draft: ProductLinksValues): SectionDiffEntry[] =>
    changedLinkKinds(original, draft).map((k) => {
      const names = (ids: string[]) => ids.map(name).join(', ')
      return { kind: 'change', label: PRODUCT_LINK_LABELS[k], before: names(original[k]), after: names(draft[k]) }
    })
}

// ---------- Content: the locale's short and full descriptions ----------

// Rich text travels as the editor's JSON, with its plain-text read beside it for the review dialog
export const productContentSchema = z.object({
  shortDescription: z.unknown(),
  shortDescriptionText: z.string(),
  descriptionRichText: z.unknown(),
  descriptionText: z.string(),
})

export type ProductContentValues = z.infer<typeof productContentSchema>

export const PRODUCT_CONTENT_LABELS: Record<keyof ProductContentValues, string> = {
  shortDescription: 'Short description',
  shortDescriptionText: 'Short description',
  descriptionRichText: 'Description',
  descriptionText: 'Description',
}

type ContentFields = { shortDescription?: unknown; descriptionRichText?: unknown }

export function toProductContentValues(product: ContentFields & { content: ({ locale: string } & ContentFields)[] }, locale: string): ProductContentValues {
  const content = product.content.find((c) => c.locale === locale)
  const shortDescription = (content?.shortDescription ?? product.shortDescription ?? null) as RichTextValue
  const descriptionRichText = (content?.descriptionRichText ?? product.descriptionRichText ?? null) as RichTextValue
  return {
    shortDescription,
    shortDescriptionText: richTextToPlainText(shortDescription),
    descriptionRichText,
    descriptionText: richTextToPlainText(descriptionRichText),
  }
}

const sameJSON = (a: unknown, b: unknown) => JSON.stringify(a ?? null) === JSON.stringify(b ?? null)

// Only the descriptions that changed - the content writer fills in the rest of the row
export function productContentChanges(before: ProductContentValues, after: ProductContentValues) {
  return {
    ...(sameJSON(before.shortDescription, after.shortDescription) ? {} : { shortDescription: after.shortDescription ?? undefined }),
    ...(sameJSON(before.descriptionRichText, after.descriptionRichText) ? {} : { descriptionRichText: after.descriptionRichText ?? undefined }),
  }
}

const clip = (s: string, max = 80) => (s.length > max ? `${s.slice(0, max - 1)}…` : s)

// Rich text shows its plain text; a formatting-only edit (same text, different JSON) still counts
function richTextEntry(label: string, beforeText: string, afterText: string, before: unknown, after: unknown) {
  if (beforeText.trim() !== afterText.trim()) return changed(label, clip(beforeText), clip(afterText))
  return sameJSON(before, after) ? null : changed(label, 'previous formatting', 'updated formatting')
}

export function productContentDiff(original: ProductContentValues, draft: ProductContentValues): SectionDiffEntry[] {
  return [
    richTextEntry(PRODUCT_CONTENT_LABELS.shortDescription, original.shortDescriptionText, draft.shortDescriptionText, original.shortDescription, draft.shortDescription),
    richTextEntry(PRODUCT_CONTENT_LABELS.descriptionRichText, original.descriptionText, draft.descriptionText, original.descriptionRichText, draft.descriptionRichText),
  ].filter((e) => e !== null)
}
