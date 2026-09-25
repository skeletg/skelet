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
  slug: z.string(),
  label: z.string(),
  helpText: z.string(),
  inputType: z.enum(INPUT_TYPES),
  isRequired: z.boolean(),
  allowComment: z.boolean(),
  sortOrder: wholeNumber(),
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

// The question's detail page - one row per locale: every stored translation, then a few missing
// locales to fill in. A stored row keeps its label; a new row stays empty until given one
const translationRow = z
  .object({
    locale: z.string(),
    label: z.string(),
    helpText: z.string(),
    // Stored already - a new row is only written once it has a label
    exists: z.boolean(),
  })
  .superRefine((row, ctx) => {
    if (row.label.trim() === '' && (row.exists || row.helpText.trim() !== '')) {
      ctx.addIssue({ code: 'custom', path: ['label'], message: 'Label is required' })
    }
  })

export const questionTranslationsSchema = z.object({ rows: z.array(translationRow) })

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
