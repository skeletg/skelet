import { z } from 'zod'
import type { SectionDiffEntry } from '@/shared/saving/saveRegistry'
import { fieldDiff } from '@/shared/saving/diff'

// Product type - the blueprint products inherit. Slug is fixed after create

export type ExistingProductType = {
  id: string
  slug: string
  name: string
  isDigital: boolean
  isShippingRequired: boolean
  urlTemplate: string
  tracksInventory: boolean
}

export const productTypeSchema = z.object({
  slug: z.string().trim().min(1, 'Slug is required'),
  name: z.string().trim().min(1, 'Name is required'),
  isDigital: z.boolean(),
  isShippingRequired: z.boolean(),
  tracksInventory: z.boolean(),
  // Checked server-side against the token grammar - empty turns auto-generation off
  urlTemplate: z.string(),
})

export type ProductTypeValues = z.infer<typeof productTypeSchema>

export function toProductTypeValues(pt?: ExistingProductType): ProductTypeValues {
  return {
    slug: pt?.slug ?? '',
    name: pt?.name ?? '',
    isDigital: pt?.isDigital ?? false,
    isShippingRequired: pt?.isShippingRequired ?? true,
    tracksInventory: pt?.tracksInventory ?? false,
    urlTemplate: pt?.urlTemplate ?? '',
  }
}

export function toProductTypeCreateInput(v: ProductTypeValues) {
  return {
    slug: v.slug.trim(),
    name: v.name.trim(),
    isDigital: v.isDigital,
    isShippingRequired: v.isShippingRequired,
    urlTemplate: v.urlTemplate,
    tracksInventory: v.tracksInventory,
  }
}

// updateProductType validates a new template but does NOT recompute the affected products' url_path -
// only the focused setProductTypeUrlPathTemplate mutation does, so a changed template is sent there
// instead and left out here
export function toProductTypeUpdateInput(v: ProductTypeValues, pt: ExistingProductType) {
  const templateChanged = v.urlTemplate !== pt.urlTemplate
  return {
    input: {
      id: pt.id,
      name: v.name.trim(),
      isDigital: v.isDigital,
      isShippingRequired: v.isShippingRequired,
      tracksInventory: v.tracksInventory,
      ...(templateChanged ? {} : { urlTemplate: v.urlTemplate }),
    },
    newTemplate: templateChanged ? v.urlTemplate : null,
  }
}

export const PRODUCT_TYPE_LABELS: Record<keyof ProductTypeValues, string> = {
  slug: 'Slug',
  name: 'Name',
  isDigital: 'Digital',
  isShippingRequired: 'Requires shipping',
  tracksInventory: 'Tracks inventory',
  urlTemplate: 'URL path template',
}

export function productTypeDiff(original: ProductTypeValues, draft: ProductTypeValues): SectionDiffEntry[] {
  return fieldDiff(
    original,
    draft,
    (['name', 'isDigital', 'isShippingRequired', 'tracksInventory', 'urlTemplate'] as const).map((k) => ({
      label: PRODUCT_TYPE_LABELS[k],
      value: (v: ProductTypeValues) => v[k],
    })),
  )
}
