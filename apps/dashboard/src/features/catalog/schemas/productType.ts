import { z } from 'zod'

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
