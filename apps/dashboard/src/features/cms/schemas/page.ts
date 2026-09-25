import { z } from 'zod'
import type { SectionDiffEntry } from '@/shared/saving/saveRegistry'
import { fieldDiff } from '@/shared/saving/diff'
import type { FieldValue } from '../DynamicFieldRenderer'
import { dynamicFieldDiff, type PageTypeAttr } from '../page-detail/shared'

// CMS page detail - its sections each stage their own form (shared/saving/useSectionForm), so the
// field lists live here one section at a time

// ---------- Basics: the slug, noindex and the non-translated attribute values ----------

export const pageBasicsSchema = z.object({
  slug: z
    .string()
    .trim()
    .min(1, 'Slug is required')
    // The backend's content slug rule (internal/content.ValidateSlug)
    .regex(/^[a-z0-9._-]+$/, 'Lowercase letters, numbers, dots, underscores and hyphens only'),
  seoNoindex: z.boolean(),
  // Attribute slug -> value, shaped by each attribute's input type (DynamicFieldRenderer)
  data: z.record(z.string(), z.unknown()),
})

export type PageBasicsValues = z.infer<typeof pageBasicsSchema>

export const PAGE_BASICS_LABELS: Record<keyof PageBasicsValues, string> = { slug: 'Slug', seoNoindex: 'Noindex', data: 'Fields' }

export function toPageBasicsValues(page: { slug: string; seoNoindex: boolean; data?: unknown }): PageBasicsValues {
  return { slug: page.slug, seoNoindex: page.seoNoindex, data: (page.data as Record<string, FieldValue> | null) ?? {} }
}

export function toPageBasicsInput(before: PageBasicsValues, v: PageBasicsValues, id: string) {
  return {
    id,
    // The service turns a changed slug into a redirect - send it only when it changed
    slug: v.slug !== before.slug ? v.slug.trim() : null,
    seoNoindex: v.seoNoindex,
    data: v.data as Record<string, FieldValue>,
  }
}

export function pageBasicsDiff(scalarAttrs: PageTypeAttr[]) {
  return (original: PageBasicsValues, draft: PageBasicsValues): SectionDiffEntry[] => [
    ...fieldDiff(original, draft, [
      { label: PAGE_BASICS_LABELS.slug, value: (v) => v.slug },
      { label: PAGE_BASICS_LABELS.seoNoindex, value: (v) => v.seoNoindex },
    ]),
    ...dynamicFieldDiff(scalarAttrs, original.data as Record<string, FieldValue>, draft.data as Record<string, FieldValue>),
  ]
}
