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
  deliveryKinds?: string[]
}

// The delivery kinds a product type can limit its products to - none ticked allows every one the channel offers
export const DELIVERY_KINDS = [
  { value: 'ship_to_address', label: 'Delivery to address' },
  { value: 'store_pickup', label: 'Store pickup' },
  { value: 'parcel_locker', label: 'Pickup point' },
  { value: 'carrier_point', label: 'Nearest pickup point' },
] as const

export const productTypeSchema = z
  .object({
    slug: z.string().trim().min(1, 'Slug is required').describe('Stable identifier of the product type - set on create, never changed'),
    name: z.string().trim().min(1, 'Name is required').describe('Display name of the product type'),
    isDigital: z.boolean().describe('Products of this type are digital goods'),
    isShippingRequired: z.boolean().describe('Products of this type need shipping'),
    tracksInventory: z.boolean().describe('Track stock levels for products of this type'),
    // Checked server-side against the token grammar
    urlTemplate: z
      .string()
      .describe('URL path template products derive their path from, such as {category}/{slug} - empty string turns auto-generation off'),
    deliveryKinds: z
      .array(z.enum(['ship_to_address', 'store_pickup', 'parcel_locker', 'carrier_point']))
      .describe('Delivery kinds products of this type may use - an empty list allows every kind the channel offers. A fridge leaves out parcel_locker and carrier_point'),
  })
  .describe('A product type: the blueprint its products inherit')

export type ProductTypeValues = z.infer<typeof productTypeSchema>

export function toProductTypeValues(pt?: ExistingProductType): ProductTypeValues {
  return {
    slug: pt?.slug ?? '',
    name: pt?.name ?? '',
    isDigital: pt?.isDigital ?? false,
    isShippingRequired: pt?.isShippingRequired ?? true,
    tracksInventory: pt?.tracksInventory ?? false,
    urlTemplate: pt?.urlTemplate ?? '',
    deliveryKinds: (pt?.deliveryKinds ?? []).filter((k): k is ProductTypeValues['deliveryKinds'][number] =>
      DELIVERY_KINDS.some((d) => d.value === k),
    ),
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
  deliveryKinds: 'Allowed delivery kinds',
}

// The kinds to send setProductTypeDeliveryKinds - null clears the limit, and nothing is sent when unchanged
export function deliveryKindsUpdate(v: ProductTypeValues, pt: ExistingProductType): string[] | null | undefined {
  const before = [...(pt.deliveryKinds ?? [])].sort().join(',')
  const after = [...v.deliveryKinds].sort().join(',')
  if (before === after) return undefined
  return v.deliveryKinds.length ? v.deliveryKinds : null
}

const deliveryKindsLabel = (kinds: string[]) =>
  kinds.length ? kinds.map((k) => DELIVERY_KINDS.find((d) => d.value === k)?.label ?? k).join(', ') : 'Every method the channel offers'

export function productTypeDiff(original: ProductTypeValues, draft: ProductTypeValues): SectionDiffEntry[] {
  return fieldDiff(
    original,
    draft,
    [
      ...(['name', 'isDigital', 'isShippingRequired', 'tracksInventory', 'urlTemplate'] as const).map((k) => ({
        label: PRODUCT_TYPE_LABELS[k],
        value: (v: ProductTypeValues) => v[k],
      })),
      { label: PRODUCT_TYPE_LABELS.deliveryKinds, value: (v: ProductTypeValues) => deliveryKindsLabel([...v.deliveryKinds].sort()) },
    ],
  )
}
