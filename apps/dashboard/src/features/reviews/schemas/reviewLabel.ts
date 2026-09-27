import { z } from 'zod'
import { wholeNumber } from '@/shared/schemas/fields'

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

export const reviewLabelSchema = z
  .object({
    slug: z
      .string()
      .trim()
      .min(1, 'Slug is required')
      .describe('Stable identifier reviews and the AI verdict key on - set on create, never changed'),
    name: z.string().trim().min(1, 'Name is required').describe('Display name of the label'),
    description: z.string().describe('Instruction the AI classifier reads to decide when a review gets this label'),
    isActive: z.boolean().describe('Whether the classifier can pick the label'),
    sortOrder: wholeNumber().describe('Order in label lists - lower first'),
  })
  .describe('A review label - a category or moderation tag the AI classifier assigns to reviews')

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
