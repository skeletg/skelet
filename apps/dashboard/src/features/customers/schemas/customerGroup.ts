import { z } from 'zod'

// Customer group dialog - slug is fixed after create

export type ExistingCustomerGroup = {
  id: string
  slug: string
  name: string
  description: string
}

export const customerGroupSchema = z.object({
  slug: z.string().trim().min(1, 'Slug is required'),
  name: z.string().trim().min(1, 'Name is required'),
  description: z.string(),
})

export type CustomerGroupValues = z.infer<typeof customerGroupSchema>

export function toCustomerGroupValues(group?: ExistingCustomerGroup): CustomerGroupValues {
  return { slug: group?.slug ?? '', name: group?.name ?? '', description: group?.description ?? '' }
}

// An emptied description is cleared, not left as an empty string
export function toCustomerGroupInput(v: CustomerGroupValues) {
  return { name: v.name.trim(), description: v.description.trim() || null }
}
