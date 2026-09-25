import { z } from 'zod'
import { wholeNumber } from '@/shared/schemas/fields'
import type { SectionDiffEntry } from '@/shared/saving/saveRegistry'
import { changed, fieldDiff } from '@/shared/saving/diff'
import { richTextToPlainText, type RichTextValue } from '../LexicalRichTextEditor'

// Category form - THE field list (docs/conventions.md, "Record forms"), for creating one and for its
// own page: the tree position, flags, and the edited locale's copy. The banner image, per-channel
// availability and storefront filters stage as their own sections; products write at once

export const categorySchema = z.object({
  name: z.string().trim().min(1, 'Name is required'),
  slug: z
    .string()
    .trim()
    .min(1, 'Slug is required')
    // The backend's content slug rule (internal/content.ValidateSlug)
    .regex(/^[a-z0-9._-]+$/, 'Lowercase letters, numbers, dots, underscores and hyphens only'),
  // '' = a root category
  parentId: z.string(),
  position: wholeNumber(),
  isPublished: z.boolean(),
  seoNoindex: z.boolean(),
  // Opaque rich-text JSON to the form - its recursive type would blow up TanStack's deep field paths
  shortDescription: z.unknown(),
  description: z.unknown(),
  metaTitle: z.string(),
  metaDescription: z.string(),
})

export type CategoryValues = z.infer<typeof categorySchema>

export const CATEGORY_LABELS: Record<keyof CategoryValues, string> = {
  name: 'Name',
  slug: 'Slug',
  parentId: 'Parent',
  position: 'Position',
  isPublished: 'Published',
  seoNoindex: 'Noindex',
  shortDescription: 'Short description',
  description: 'Description',
  metaTitle: 'Meta title',
  metaDescription: 'Meta description',
}

export const emptyCategory: CategoryValues = {
  name: '',
  slug: '',
  parentId: '',
  position: 0,
  isPublished: false,
  seoNoindex: false,
  shortDescription: null,
  description: null,
  metaTitle: '',
  metaDescription: '',
}

type ExistingCategory = {
  slug: string
  name?: string | null
  position: number
  isPublished: boolean
  seoNoindex: boolean
  parent?: { id: string } | null
  content: { locale: string; name: string; shortDescription?: unknown; description?: unknown; metaTitle?: string | null; metaDescription?: string | null }[]
}

// The copy is the edited locale's row - falling back to the first one keeps a category with no
// translation for this locale editable
export function toCategoryValues(c: ExistingCategory, locale: string): CategoryValues {
  const content = c.content.find((e) => e.locale === locale) ?? c.content[0]
  return {
    name: content?.name ?? c.name ?? '',
    slug: c.slug,
    parentId: c.parent?.id ?? '',
    position: c.position,
    isPublished: c.isPublished,
    seoNoindex: c.seoNoindex,
    shortDescription: content?.shortDescription ?? null,
    description: content?.description ?? null,
    metaTitle: content?.metaTitle ?? '',
    metaDescription: content?.metaDescription ?? '',
  }
}

const contentInput = (v: CategoryValues) => ({
  name: v.name.trim(),
  shortDescription: (v.shortDescription as RichTextValue) ?? null,
  description: (v.description as RichTextValue) ?? null,
  metaTitle: v.metaTitle.trim() || null,
  metaDescription: v.metaDescription.trim() || null,
})

const structureInput = (v: CategoryValues) => ({
  slug: v.slug.trim(),
  position: v.position,
  isPublished: v.isPublished,
  seoNoindex: v.seoNoindex,
})

export function toCategoryCreateInput(v: CategoryValues, locale: string) {
  return { parentId: v.parentId || null, locale, ...structureInput(v), ...contentInput(v) }
}

const sameJson = (a: unknown, b: unknown) => JSON.stringify(a ?? null) === JSON.stringify(b ?? null)

// The writes an edit needs: the category's own update and the locale's content row, each only when
// its part changed. The content upsert replaces the row, so it always carries every content field
export function categoryChanges(before: CategoryValues, after: CategoryValues) {
  const structure =
    before.slug !== after.slug ||
    before.parentId !== after.parentId ||
    before.position !== after.position ||
    before.isPublished !== after.isPublished ||
    before.seoNoindex !== after.seoNoindex
  const content =
    before.name !== after.name ||
    before.metaTitle !== after.metaTitle ||
    before.metaDescription !== after.metaDescription ||
    !sameJson(before.shortDescription, after.shortDescription) ||
    !sameJson(before.description, after.description)
  return {
    update: structure
      ? { ...structureInput(after), ...(after.parentId ? { parentId: after.parentId } : { clearParent: true }) }
      : null,
    content: content ? contentInput(after) : null,
  }
}

const clip = (s: string, max = 80) => (s.length > max ? `${s.slice(0, max - 1)}…` : s)

// Rich text reads as its plain text; an edit that only changes the formatting still shows up
function richTextEntry(label: string, before: unknown, after: unknown): SectionDiffEntry | null {
  if (sameJson(before, after)) return null
  const b = richTextToPlainText(before as RichTextValue)
  const a = richTextToPlainText(after as RichTextValue)
  return b === a ? changed(label, 'previous formatting', 'updated formatting') : changed(label, clip(b), clip(a))
}

export function categoryDiff(parentName: (id: string) => string) {
  return (original: CategoryValues, draft: CategoryValues): SectionDiffEntry[] => [
    ...fieldDiff(original, draft, [
      { label: CATEGORY_LABELS.name, value: (v) => v.name },
      { label: CATEGORY_LABELS.slug, value: (v) => v.slug },
      { label: CATEGORY_LABELS.parentId, value: (v) => (v.parentId ? parentName(v.parentId) : 'None (root)') },
      { label: CATEGORY_LABELS.position, value: (v) => v.position },
      { label: CATEGORY_LABELS.isPublished, value: (v) => v.isPublished },
      { label: CATEGORY_LABELS.seoNoindex, value: (v) => v.seoNoindex },
      { label: CATEGORY_LABELS.metaTitle, value: (v) => v.metaTitle },
      { label: CATEGORY_LABELS.metaDescription, value: (v) => clip(v.metaDescription) },
    ]),
    ...[
      richTextEntry(CATEGORY_LABELS.shortDescription, original.shortDescription, draft.shortDescription),
      richTextEntry(CATEGORY_LABELS.description, original.description, draft.description),
    ].filter((e) => e !== null),
  ]
}
