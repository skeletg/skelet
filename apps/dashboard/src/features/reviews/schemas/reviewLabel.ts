import { z } from 'zod'

// Review categories and moderation tags are the same record - a label the AI classifier picks per
// review. The slug is fixed after create (reviews and the AI verdict key on it); the description is
// the instruction the classifier reads, so it matters more than the name

export type ExistingReviewLabel = {
  id: string
  slug: string
  name: string
  description: string
  isActive: boolean
  sortOrder: number
}

export const reviewLabelSchema = z.object({
  slug: z.string().trim().min(1, 'Slug is required'),
  name: z.string().trim().min(1, 'Name is required'),
  description: z.string(),
  isActive: z.boolean(),
  sortOrder: z.number().int('Whole numbers only'),
})

export type ReviewLabelValues = z.infer<typeof reviewLabelSchema>

export function toReviewLabelValues(label?: ExistingReviewLabel | null): ReviewLabelValues {
  return {
    slug: label?.slug ?? '',
    name: label?.name ?? '',
    description: label?.description ?? '',
    isActive: label?.isActive ?? true,
    sortOrder: label?.sortOrder ?? 0,
  }
}

export function toReviewLabelInput(v: ReviewLabelValues) {
  return {
    name: v.name.trim(),
    description: v.description.trim() || null,
    isActive: v.isActive,
    sortOrder: v.sortOrder,
  }
}
