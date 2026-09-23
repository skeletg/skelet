import { z } from 'zod'
import type { ReviewQuestionInputType } from '@/graphql/schema.types'
import type { ReviewQuestionFieldsFragment } from '../Reviews.gql'

// Review question dialog. Slug and input type are fixed after create - changing them would orphan
// existing answer rows - so an edit only touches the runtime knobs, and the label is seeded once
// for the default locale (translations live on the question's detail page)

export const INPUT_TYPES = ['RATING_1_5', 'YES_NO', 'FREE_TEXT'] as const satisfies readonly ReviewQuestionInputType[]

const shape = {
  slug: z.string(),
  label: z.string(),
  helpText: z.string(),
  inputType: z.enum(INPUT_TYPES),
  isRequired: z.boolean(),
  allowComment: z.boolean(),
  sortOrder: z.number().int('Whole numbers only'),
}

// Create seeds the label and slug; edit never shows them, so it mustn't block on them
export const createReviewQuestionSchema = z.object({
  ...shape,
  slug: z.string().trim().min(1, 'Slug is required'),
  label: z.string().trim().min(1, 'Label is required'),
})
export const editReviewQuestionSchema = z.object(shape)

export type ReviewQuestionValues = z.infer<typeof editReviewQuestionSchema>

export function toReviewQuestionValues(q?: ReviewQuestionFieldsFragment | null): ReviewQuestionValues {
  return {
    slug: q?.slug ?? '',
    label: q?.content?.label ?? '',
    helpText: q?.content?.helpText ?? '',
    inputType: q?.inputType ?? 'RATING_1_5',
    isRequired: q?.isRequired ?? false,
    allowComment: q?.allowComment ?? true,
    sortOrder: q?.sortOrder ?? 0,
  }
}

export function toReviewQuestionCreateInput(v: ReviewQuestionValues, locale: string) {
  return {
    slug: v.slug.trim(),
    inputType: v.inputType,
    isRequired: v.isRequired,
    allowComment: v.allowComment,
    sortOrder: v.sortOrder,
    locale,
    label: v.label.trim(),
    helpText: v.helpText.trim() || null,
  }
}

export function toReviewQuestionUpdateInput(v: ReviewQuestionValues, id: string) {
  return { id, isRequired: v.isRequired, allowComment: v.allowComment, sortOrder: v.sortOrder }
}
