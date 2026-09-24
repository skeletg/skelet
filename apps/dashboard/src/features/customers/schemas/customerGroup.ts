import { z } from 'zod'
import type { SectionDiffEntry } from '@/shared/saving/saveRegistry'
import { fieldDiff } from '@/shared/saving/diff'

// Customer group - THE field list (docs/conventions.md, "Record forms"), for the create/edit dialog
// and the group's own page. Slug is fixed after create

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

export const CUSTOMER_GROUP_LABELS: Record<keyof CustomerGroupValues, string> = {
  slug: 'Slug',
  name: 'Name',
  description: 'Description',
}

// The group's own page - the slug never changes there, so only name and description can differ
export function customerGroupDiff(original: CustomerGroupValues, draft: CustomerGroupValues): SectionDiffEntry[] {
  return fieldDiff(original, draft, [
    { label: CUSTOMER_GROUP_LABELS.name, value: (v) => v.name },
    { label: CUSTOMER_GROUP_LABELS.description, value: (v) => v.description },
  ])
}
