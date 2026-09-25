import { z } from 'zod'
import type { SectionDiffEntry } from '@/shared/saving/saveRegistry'
import { changed, fieldDiff } from '@/shared/saving/diff'
import { formatCents } from '@/lib/format'

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
