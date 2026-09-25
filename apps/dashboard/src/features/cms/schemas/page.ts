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

// ---------- Selections and relationships: value ids per attribute ----------

// Attribute id -> the chosen value ids (selections) or linked entity ids (relationships, in order).
// One write per changed attribute, so they're grouped by attribute rather than flattened
export const pageLinksSchema = z.object({ byAttribute: z.record(z.string(), z.array(z.string())) })

export type PageLinksValues = z.infer<typeof pageLinksSchema>

export const PAGE_LINKS_LABELS: Record<keyof PageLinksValues, string> = { byAttribute: 'Values' }

export function toPageLinksValues(attrs: PageTypeAttr[], rows: { attributeId: string; id: string }[]): PageLinksValues {
  return {
    byAttribute: Object.fromEntries(attrs.map((pta) => [pta.attribute.id, rows.filter((r) => r.attributeId === pta.attribute.id).map((r) => r.id)])),
  }
}

// `ordered` - relationships keep their order, so a reorder is a change; selections are a set
export function changedAttributes(before: PageLinksValues, after: PageLinksValues, ordered: boolean): string[] {
  const norm = (ids: string[] | undefined) => (ordered ? (ids ?? []) : [...(ids ?? [])].sort())
  return Object.keys(after.byAttribute).filter((id) => JSON.stringify(norm(before.byAttribute[id])) !== JSON.stringify(norm(after.byAttribute[id])))
}

const attrName = (attrs: PageTypeAttr[], id: string) => {
  const a = attrs.find((pta) => pta.attribute.id === id)?.attribute
  return a ? (a.content?.name ?? a.slug) : id
}

// Selections read as the chosen values' slugs
export function pageSelectionsDiff(attrs: PageTypeAttr[]) {
  return (original: PageLinksValues, draft: PageLinksValues): SectionDiffEntry[] =>
    changedAttributes(original, draft, false).map((id) => {
      const values = attrs.find((pta) => pta.attribute.id === id)?.attribute.values ?? []
      const names = (ids: string[] | undefined) => (ids ?? []).map((v) => values.find((x) => x.id === v)?.slug ?? v).join(', ')
      return { kind: 'change', label: attrName(attrs, id), before: names(original.byAttribute[id]), after: names(draft.byAttribute[id]) }
    })
}

// Relationships read as how many are linked - the ids alone mean nothing to the admin
export function pageRelationshipsDiff(attrs: PageTypeAttr[]) {
  const count = (ids: string[] | undefined) => `${(ids ?? []).length} linked`
  return (original: PageLinksValues, draft: PageLinksValues): SectionDiffEntry[] =>
    changedAttributes(original, draft, true).map((id) => {
      const [b, a] = [count(original.byAttribute[id]), count(draft.byAttribute[id])]
      return { kind: 'change', label: attrName(attrs, id), before: b, after: a === b ? `${a}, reordered or swapped` : a }
    })
}
