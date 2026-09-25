import { z } from 'zod'
import type { SectionDiffEntry } from '@/shared/saving/saveRegistry'
import { added, changed, fieldDiff, removed } from '@/shared/saving/diff'
import { formatCents } from '@/lib/format'
import { metadataDiff, metadataSchema, toMetadataValues, type MetadataEntry } from '@/shared/schemas/metadata'
import { centsToDollars, dollarsToCents, isNumberText, money, numberText } from '@/shared/schemas/fields'
import { richTextToPlainText, type RichTextValue } from '../LexicalRichTextEditor'

// Product page - its sections each stage their own form (shared/saving/useSectionForm), so the field
// lists live here one section at a time

// ---------- General: the locale's name, SKU, tax class and default price ----------

export const productGeneralSchema = z.object({
  name: z.string().trim().min(1, 'Name is required'),
  sku: z.string(),
  // Blank means the default class
  taxClass: z.string(),
  priceDollars: money(),
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
const blankOrMoney = money({ optional: true })
const blankOrCount = numberText({ optional: true, min: 0 })

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
    if (!isNumberText(r.minQty, { above: 0 })) {
      ctx.addIssue({ code: 'custom', path: ['tiers', i, 'minQty'], message: 'More than zero' })
    } else if (seen.has(tierKey(r))) {
      ctx.addIssue({ code: 'custom', path: ['tiers', i, 'minQty'], message: 'This group already has a tier at this quantity' })
    }
    seen.add(tierKey(r))
    if (!isNumberText(r.price, { min: 0 })) {
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

// ---------- SEO: the URL, canonical and noindex on the product, the locale's meta copy ----------

export const productSeoSchema = z.object({
  // On, the URL comes from the template; off, `url` is the full path (blank = generate once from the name)
  urlAuto: z.boolean(),
  url: z.string(),
  urlTemplateOverride: z.string(),
  canonicalUrl: z.string(),
  seoNoindex: z.boolean(),
  metaTitle: z.string(),
  metaDescription: z.string(),
  metaKeywords: z.string(),
})

export type ProductSeoValues = z.infer<typeof productSeoSchema>

export const PRODUCT_SEO_LABELS: Record<keyof ProductSeoValues, string> = {
  urlAuto: 'Auto-generate URL',
  url: 'URL',
  urlTemplateOverride: 'URL template override',
  canonicalUrl: 'Canonical URL',
  seoNoindex: 'Noindex',
  metaTitle: 'Meta title',
  metaDescription: 'Meta description',
  metaKeywords: 'Meta keywords',
}

type SeoProduct = {
  url?: string | null
  urlAuto: boolean
  urlTemplateOverride?: string | null
  canonicalUrl?: string | null
  seoNoindex: boolean
  metaTitle?: string | null
  metaDescription?: string | null
  content: { locale: string; metaTitle?: string | null; metaDescription?: string | null; metaKeywords?: string | null }[]
}

export function toProductSeoValues(product: SeoProduct, locale: string): ProductSeoValues {
  const content = product.content.find((c) => c.locale === locale)
  return {
    urlAuto: product.urlAuto,
    url: product.url ?? '',
    urlTemplateOverride: product.urlTemplateOverride ?? '',
    canonicalUrl: product.canonicalUrl ?? '',
    seoNoindex: product.seoNoindex,
    metaTitle: content?.metaTitle ?? product.metaTitle ?? '',
    metaDescription: content?.metaDescription ?? product.metaDescription ?? '',
    metaKeywords: content?.metaKeywords ?? '',
  }
}

const META = ['metaTitle', 'metaDescription', 'metaKeywords'] as const

// The product update carries only what changed (nil is "no change" there, '' clears); the meta copy
// goes to the locale's content row. A manual URL is sent only with auto off - the backend refuses one
// while auto is on
export function productSeoChanges(before: ProductSeoValues, after: ProductSeoValues) {
  const t = (s: string) => s.trim()
  const update = {
    ...(before.urlAuto !== after.urlAuto ? { urlAuto: after.urlAuto } : {}),
    ...(!after.urlAuto && (before.urlAuto || t(before.url) !== t(after.url)) ? { url: t(after.url) } : {}),
    ...(t(before.urlTemplateOverride) !== t(after.urlTemplateOverride) ? { urlTemplateOverride: t(after.urlTemplateOverride) } : {}),
    ...(t(before.canonicalUrl) !== t(after.canonicalUrl) ? { canonicalUrl: t(after.canonicalUrl) } : {}),
    ...(before.seoNoindex !== after.seoNoindex ? { seoNoindex: after.seoNoindex } : {}),
  }
  return {
    update: Object.keys(update).length > 0 ? update : null,
    content: META.some((k) => t(before[k]) !== t(after[k])) ? { metaTitle: t(after.metaTitle), metaDescription: t(after.metaDescription), metaKeywords: t(after.metaKeywords) } : null,
  }
}

export function productSeoDiff(original: ProductSeoValues, draft: ProductSeoValues): SectionDiffEntry[] {
  return fieldDiff(original, draft, [
    { label: PRODUCT_SEO_LABELS.urlAuto, value: (v) => (v.urlAuto ? 'on' : 'off') },
    // The manual URL only means something with auto off
    { label: PRODUCT_SEO_LABELS.url, value: (v) => (v.urlAuto ? original.url : v.url) },
    { label: PRODUCT_SEO_LABELS.urlTemplateOverride, value: (v) => v.urlTemplateOverride },
    { label: PRODUCT_SEO_LABELS.canonicalUrl, value: (v) => v.canonicalUrl },
    { label: PRODUCT_SEO_LABELS.seoNoindex, value: (v) => v.seoNoindex },
    { label: PRODUCT_SEO_LABELS.metaTitle, value: (v) => v.metaTitle },
    { label: PRODUCT_SEO_LABELS.metaDescription, value: (v) => clip(v.metaDescription) },
    { label: PRODUCT_SEO_LABELS.metaKeywords, value: (v) => v.metaKeywords },
  ])
}

// ---------- Per-store URLs: a store's own path for the product, blank inherits ----------

export const productChannelUrlsSchema = z.object({
  // channel id -> path
  urls: z.record(z.string(), z.string()),
})

export type ProductChannelUrlsValues = z.infer<typeof productChannelUrlsSchema>

export const PRODUCT_CHANNEL_URL_LABELS: Record<keyof ProductChannelUrlsValues, string> = { urls: 'Per-store URLs' }

export function toProductChannelUrlValues(listings: { url?: string | null; channel: { id: string } }[]): ProductChannelUrlsValues {
  return { urls: Object.fromEntries(listings.map((l) => [l.channel.id, l.url ?? ''])) }
}

// The stores whose URL changed, with what to send - null clears the override
export function channelUrlChanges(before: ProductChannelUrlsValues, after: ProductChannelUrlsValues) {
  return Object.entries(after.urls)
    .filter(([id, url]) => url.trim() !== (before.urls[id] ?? '').trim())
    .map(([channelId, url]) => ({ channelId, url: url.trim() || null }))
}

export function productChannelUrlsDiff(channelName: (id: string) => string) {
  return (original: ProductChannelUrlsValues, draft: ProductChannelUrlsValues): SectionDiffEntry[] =>
    channelUrlChanges(original, draft).map(({ channelId, url }) => ({
      kind: 'change',
      label: `${channelName(channelId)} URL`,
      before: original.urls[channelId]?.trim() || 'inherit',
      after: url ?? 'inherit',
    }))
}

// ---------- Per-store pricing & publishing: each store's price and published flag ----------

const storeListing = z.object({
  channelId: z.string(),
  isPublished: z.boolean(),
  // Off, `price` is the store's own amount; on, the store takes the product's default
  inheritPrice: z.boolean(),
  price: z.string(),
  // Staged "Clear" - the listing goes, back to inherited and unpublished
  clear: z.boolean(),
})

export type ProductStoreListing = z.infer<typeof storeListing>

export const productListingsSchema = z.object({ stores: z.array(storeListing) }).superRefine((v, ctx) => {
  v.stores.forEach((s, i) => {
    if (!s.clear && !s.inheritPrice && !isNumberText(s.price, { min: 0 })) {
      ctx.addIssue({ code: 'custom', path: ['stores', i, 'price'], message: 'Zero or more, or use the default' })
    }
  })
})

export type ProductListingsValues = z.infer<typeof productListingsSchema>

export const PRODUCT_LISTING_LABELS: Record<keyof ProductListingsValues, string> = { stores: 'Per-store pricing & publishing' }

type ExistingListing = { isPublished: boolean; price?: number | null; channel: { id: string } }

export function toProductListingValues(channelIds: string[], listings: ExistingListing[]): ProductListingsValues {
  const byId = new Map(listings.map((l) => [l.channel.id, l]))
  return {
    stores: channelIds.map((channelId) => {
      const l = byId.get(channelId)
      return { channelId, isPublished: l?.isPublished ?? false, inheritPrice: l?.price == null, price: l?.price != null ? centsToDollars(l.price) : '', clear: false }
    }),
  }
}

export const clearedStore = (channelId: string): ProductStoreListing => ({ channelId, isPublished: false, inheritPrice: true, price: '', clear: true })

const sameStore = (a: ProductStoreListing, b: ProductStoreListing) =>
  a.clear === b.clear && a.isPublished === b.isPublished && a.inheritPrice === b.inheritPrice && (a.inheritPrice || moneyText(a.price) === moneyText(b.price))

type ListingChange = { channelId: string; clear: true } | { channelId: string; clear: false; isPublished: boolean; price: number | null }

// What each changed store needs: a clear (only when it had a listing to clear) or a write
export function productListingChanges(before: ProductListingsValues, after: ProductListingsValues, hasListing: (id: string) => boolean): ListingChange[] {
  return after.stores.flatMap((s): ListingChange[] => {
    const was = before.stores.find((b) => b.channelId === s.channelId)
    if (!was || sameStore(was, s)) return []
    if (s.clear) return hasListing(s.channelId) ? [{ channelId: s.channelId, clear: true }] : []
    return [{ channelId: s.channelId, clear: false, isPublished: s.isPublished, price: s.inheritPrice ? null : dollarsToCents(s.price) }]
  })
}

export function productListingsDiff(store: (id: string) => { name: string; currencyCode: string }, hasListing: (id: string) => boolean) {
  return (original: ProductListingsValues, draft: ProductListingsValues): SectionDiffEntry[] => {
    const describe = (s: ProductStoreListing | undefined) =>
      s
        ? `${s.inheritPrice ? 'inherit default' : formatCents(dollarsToCents(s.price), store(s.channelId).currencyCode)} · ${s.isPublished ? 'published' : 'unpublished'}`
        : ''
    const find = (v: ProductListingsValues, id: string) => v.stores.find((s) => s.channelId === id)
    return productListingChanges(original, draft, hasListing).map(({ channelId, clear }) => ({
      kind: 'change',
      label: store(channelId).name,
      before: describe(find(original, channelId)),
      after: clear ? 'cleared (revert to default)' : describe(find(draft, channelId)),
    }))
  }
}

// ---------- Variants: the variant mode and the configurable option attributes ----------

export const productVariantsSchema = z.object({
  mode: z.enum(['ATTRIBUTE', 'LIST']),
  // Attribute ids - a set, only meaningful in ATTRIBUTE mode
  options: z.array(z.string()),
})

export type ProductVariantsValues = z.infer<typeof productVariantsSchema>

export const PRODUCT_VARIANT_LABELS: Record<keyof ProductVariantsValues, string> = { mode: 'Variant mode', options: 'Configurable options' }

export const VARIANT_MODE_LABELS: Record<ProductVariantsValues['mode'], string> = {
  ATTRIBUTE: 'Attribute-based',
  LIST: 'List (no attributes)',
}

export function toProductVariantValues(product: { variantMode?: ProductVariantsValues['mode'] | null; configurableOptions: { id: string }[] }): ProductVariantsValues {
  return { mode: product.variantMode ?? 'ATTRIBUTE', options: product.configurableOptions.map((a) => a.id) }
}

// Mode first when both change: setConfigurableOptions is refused while the product is still LIST
export function productVariantChanges(before: ProductVariantsValues, after: ProductVariantsValues) {
  const optionsChanged = after.mode === 'ATTRIBUTE' && (before.options.length !== after.options.length || after.options.some((id) => !before.options.includes(id)))
  return { mode: before.mode !== after.mode ? after.mode : null, options: optionsChanged ? after.options : null }
}

export function productVariantsDiff(attributeName: (id: string) => string) {
  return (original: ProductVariantsValues, draft: ProductVariantsValues): SectionDiffEntry[] => {
    const changes = productVariantChanges(original, draft)
    return [
      ...(changes.mode ? [changed('Variant mode', VARIANT_MODE_LABELS[original.mode], VARIANT_MODE_LABELS[changes.mode])] : []),
      ...(changes.options
        ? [
            ...changes.options.filter((id) => !original.options.includes(id)).map((id) => added('Option', attributeName(id))),
            ...original.options.filter((id) => !changes.options?.includes(id)).map((id) => removed('Option', attributeName(id))),
          ]
        : []),
    ].filter((e) => e !== null)
  }
}

// ---------- Metadata: the public and private key/value lists ----------

// A value typed without a key would be dropped on save, and a key twice would lose one of the two -
// both are refused instead
const keyedMetadata = metadataSchema.superRefine((rows, ctx) => {
  const seen = new Set<string>()
  for (const r of rows) {
    const key = r.key.trim()
    if (!key) {
      if (r.value.trim()) ctx.addIssue({ code: 'custom', message: 'Every value needs a key' })
      continue
    }
    if (seen.has(key)) ctx.addIssue({ code: 'custom', message: `"${key}" is used twice` })
    seen.add(key)
  }
})

export const productMetadataSchema = z.object({ metadata: keyedMetadata, privateMetadata: keyedMetadata })

export type ProductMetadataValues = z.infer<typeof productMetadataSchema>

export const PRODUCT_METADATA_LABELS: Record<keyof ProductMetadataValues, string> = { metadata: 'Metadata', privateMetadata: 'Private metadata' }

export function toProductMetadataValues(product: { metadata: MetadataEntry[]; privateMetadata: MetadataEntry[] }): ProductMetadataValues {
  return { metadata: toMetadataValues(product.metadata), privateMetadata: toMetadataValues(product.privateMetadata) }
}

export function productMetadataDiff(original: ProductMetadataValues, draft: ProductMetadataValues): SectionDiffEntry[] {
  return [...metadataDiff(original.metadata, draft.metadata), ...metadataDiff(original.privateMetadata, draft.privateMetadata, 'Private metadata')]
}

// ---------- Categories: which categories the product sits in, and its primary one ----------

export const productCategoriesSchema = z
  .object({
    assigned: z.array(z.string()),
    // '' = none - only when nothing is assigned
    primaryId: z.string(),
  })
  .superRefine((v, ctx) => {
    if (v.assigned.length > 0 && !v.assigned.includes(v.primaryId)) {
      ctx.addIssue({ code: 'custom', path: ['primaryId'], message: 'Star one of the assigned categories as primary' })
    }
  })

export type ProductCategoriesValues = z.infer<typeof productCategoriesSchema>

export const PRODUCT_CATEGORY_LABELS: Record<keyof ProductCategoriesValues, string> = { assigned: 'Categories', primaryId: 'Primary category' }

export function toProductCategoryValues(categories: { id: string }[], primary: { id: string } | null): ProductCategoriesValues {
  return { assigned: categories.map((c) => c.id), primaryId: primary?.id ?? '' }
}

// Assigning keeps a primary: the first category assigned becomes it, and unassigning the primary
// hands it to another assigned one
export function toggleCategory(v: ProductCategoriesValues, id: string): ProductCategoriesValues {
  if (v.assigned.includes(id)) {
    const assigned = v.assigned.filter((x) => x !== id)
    return { assigned, primaryId: v.primaryId === id ? (assigned[0] ?? '') : v.primaryId }
  }
  return { assigned: [...v.assigned, id], primaryId: v.primaryId || id }
}

export function productCategoryChanges(before: ProductCategoriesValues, after: ProductCategoriesValues) {
  return {
    add: after.assigned.filter((id) => !before.assigned.includes(id)),
    remove: before.assigned.filter((id) => !after.assigned.includes(id)),
    primary: after.primaryId && after.primaryId !== before.primaryId ? after.primaryId : null,
  }
}

export function productCategoriesDiff(name: (id: string) => string) {
  return (original: ProductCategoriesValues, draft: ProductCategoriesValues): SectionDiffEntry[] => {
    const { add, remove } = productCategoryChanges(original, draft)
    const primary = original.primaryId !== draft.primaryId ? changed('Primary', original.primaryId && name(original.primaryId), draft.primaryId && name(draft.primaryId)) : null
    return [...add.map((id) => added('Category', name(id))), ...remove.map((id) => removed('Category', name(id))), ...(primary ? [primary] : [])]
  }
}
