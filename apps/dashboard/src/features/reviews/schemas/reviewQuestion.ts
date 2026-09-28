import { z } from 'zod'
import { wholeNumber } from '@/shared/schemas/fields'
import type { ReviewQuestionInputType } from '@/graphql/schema.types'
import type { ReviewQuestionFieldsFragment } from '../Reviews.gql'
import type { SectionDiffEntry } from '@/shared/saving/saveRegistry'
import { changed } from '@/shared/saving/diff'

// Review question dialog. Slug and input type are fixed after create - changing them would orphan
// existing answer rows - so an edit only touches the runtime knobs, and the label is seeded once
// for the default locale (translations live on the question's detail page)

export const INPUT_TYPES = ['RATING_1_5', 'YES_NO', 'FREE_TEXT'] as const satisfies readonly ReviewQuestionInputType[]

const shape = {
  slug: z.string().describe('Stable identifier of the question - set on create, never changed'),
  label: z.string().describe('Question text in the default locale - set on create'),
  helpText: z.string().describe('Hint shown under the question in the default locale - empty string for none'),
  inputType: z
    .enum(INPUT_TYPES)
    .describe('How shoppers answer: RATING_1_5 stars, YES_NO, or FREE_TEXT - set on create, never changed'),
  isRequired: z.boolean().describe('Shoppers must answer the question'),
  allowComment: z.boolean().describe('Shoppers can add a comment to their answer'),
  sortOrder: wholeNumber().describe('Order on the review form - lower first'),
}

// Create seeds the label and slug; edit never shows them, so it mustn't block on them
export const createReviewQuestionSchema = z
  .object({
    ...shape,
    slug: z.string().trim().min(1, 'Slug is required').describe('Stable identifier of the question - never changed'),
    label: z.string().trim().min(1, 'Label is required').describe('Question text in the default locale'),
  })
  .describe('A new question on the product review form')
export const editReviewQuestionSchema = z.object(shape).describe('Changes to a review question')

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

// The question's detail page - one row per locale: every stored translation, then a few missing
// locales to fill in. A stored row keeps its label; a new row stays empty until given one
const translationRow = z
  .object({
    locale: z.string().describe('Locale code of the translation, such as en-US'),
    label: z.string().describe('Question text in this locale - required once the row is stored or has help text'),
    helpText: z.string().describe('Hint shown under the question in this locale - empty string for none'),
    // A new row is only written once it has a label
    exists: z.boolean().describe('The translation is already stored'),
  })
  .describe("One locale's translation of a review question")
  .superRefine((row, ctx) => {
    if (row.label.trim() === '' && (row.exists || row.helpText.trim() !== '')) {
      ctx.addIssue({ code: 'custom', path: ['label'], message: 'Label is required' })
    }
  })

export const questionTranslationsSchema = z
  .object({ rows: z.array(translationRow).describe('One row per locale') })
  .describe("A review question's translations")

export type QuestionTranslationsValues = z.infer<typeof questionTranslationsSchema>
type TranslationRow = QuestionTranslationsValues['rows'][number]

export const QUESTION_TRANSLATIONS_LABELS = { rows: 'Translations' }

export function toQuestionTranslationsValues(
  question: Pick<ReviewQuestionFieldsFragment, 'allContent'>,
  newLocales: string[],
): QuestionTranslationsValues {
  return {
    rows: [
      ...question.allContent.map((c) => ({ locale: c.locale, label: c.label, helpText: c.helpText, exists: true })),
      ...newLocales.map((locale) => ({ locale, label: '', helpText: '', exists: false })),
    ],
  }
}

// The rows worth writing - rows are matched by locale, so a reordered baseline can't misfire
export function changedTranslations(before: QuestionTranslationsValues, after: QuestionTranslationsValues): TranslationRow[] {
  return after.rows.filter((row) => {
    const was = before.rows.find((r) => r.locale === row.locale)
    return !was || was.label !== row.label || was.helpText !== row.helpText
  })
}

export function toTranslationInput(questionId: string, row: TranslationRow) {
  return { questionId, locale: row.locale, label: row.label.trim(), helpText: row.helpText.trim() || null }
}

export function questionTranslationsDiff(before: QuestionTranslationsValues, after: QuestionTranslationsValues): SectionDiffEntry[] {
  return changedTranslations(before, after).flatMap((row) => {
    const was = before.rows.find((r) => r.locale === row.locale)
    return [
      changed(`Label (${row.locale})`, was?.label, row.label),
      changed(`Help text (${row.locale})`, was?.helpText, row.helpText),
    ].filter((e) => e !== null)
  })
}
