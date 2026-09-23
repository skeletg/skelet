import { z } from 'zod'
import type { SectionDiffEntry } from '@/shared/saving/saveRegistry'
import { fieldDiff } from '@/shared/saving/diff'

// A blog taxonomy term - categories and tags are the same record: a slug, a published flag and a
// per-locale name. Tags may leave the name empty (it falls back to the slug); categories may not

export type ExistingTerm = {
  id: string
  slug: string
  isPublished: boolean
  content?: { name: string } | null
  allContent: { locale: string; name: string }[]
}

export function termSchema(opts: { nameRequired: boolean }) {
  return z.object({
    slug: z.string().trim().min(1, 'Slug is required'),
    name: opts.nameRequired ? z.string().trim().min(1, 'Name is required') : z.string(),
    isPublished: z.boolean(),
  })
}

export type TermValues = z.infer<ReturnType<typeof termSchema>>

export const TERM_LABELS: Record<keyof TermValues, string> = {
  slug: 'Slug',
  name: 'Name',
  isPublished: 'Published',
}

// The name shown for the locale being edited - its own row, else the first one stored
export function currentTermName(term?: ExistingTerm): string {
  return term?.content?.name ?? term?.allContent[0]?.name ?? ''
}

export function toTermValues(term: ExistingTerm | undefined, defaultPublished: boolean): TermValues {
  return {
    slug: term?.slug ?? '',
    name: currentTermName(term),
    isPublished: term?.isPublished ?? defaultPublished,
  }
}

export function termDiff(original: TermValues, draft: TermValues): SectionDiffEntry[] {
  return fieldDiff(original, draft, [
    { label: TERM_LABELS.slug, value: (o) => o.slug },
    { label: TERM_LABELS.name, value: (o) => o.name },
    { label: TERM_LABELS.isPublished, value: (o) => o.isPublished },
  ])
}
