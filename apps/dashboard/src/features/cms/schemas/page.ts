import { z } from 'zod'
import { contentSlug } from '@/shared/schemas/fields'
import type { SectionDiffEntry } from '@/shared/saving/saveRegistry'
import { fieldDiff } from '@/shared/saving/diff'
import type { FieldValue } from '../DynamicFieldRenderer'
import { dynamicFieldDiff, type PageTypeAttr } from '../page-detail/shared'

// CMS page detail - its sections each stage their own form (shared/saving/useSectionForm), so the
// field lists live here one section at a time

// ---------- Basics: the slug and the non-translated attribute values ----------
// Search indexing is the SEO panel's alone - a second copy here staged its own value and the two saves overwrote each other

export const pageBasicsSchema = z
  .object({
    slug: contentSlug().describe(
      'URL slug of the page - a change leaves a redirect behind. Lowercase letters, numbers, dots, underscores and hyphens',
    ),
    // Shaped per input type by DynamicFieldRenderer
    data: z
      .record(z.string(), z.unknown())
      .describe("Values of the page type's non-translatable attributes, keyed by attribute slug"),
  })
  .describe("A CMS page's slug and non-translatable field values")

export type PageBasicsValues = z.infer<typeof pageBasicsSchema>

export const PAGE_BASICS_LABELS: Record<keyof PageBasicsValues, string> = { slug: 'Slug', data: 'Fields' }

export function toPageBasicsValues(page: { slug: string; data?: unknown }): PageBasicsValues {
  return { slug: page.slug, data: (page.data as Record<string, FieldValue> | null) ?? {} }
}

export function toPageBasicsInput(before: PageBasicsValues, v: PageBasicsValues, id: string) {
  return {
    id,
    // The service turns a changed slug into a redirect - send it only when it changed
    slug: v.slug !== before.slug ? v.slug.trim() : null,
    data: v.data as Record<string, FieldValue>,
  }
}

export function pageBasicsDiff(scalarAttrs: PageTypeAttr[]) {
  return (original: PageBasicsValues, draft: PageBasicsValues): SectionDiffEntry[] => [
    ...fieldDiff(original, draft, [{ label: PAGE_BASICS_LABELS.slug, value: (v) => v.slug }]),
    ...dynamicFieldDiff(scalarAttrs, original.data as Record<string, FieldValue>, draft.data as Record<string, FieldValue>),
  ]
}

// ---------- Selections and relationships: value ids per attribute ----------

// Attribute id -> the chosen value ids (selections) or linked entity ids (relationships, in order).
// One write per changed attribute, so they're grouped by attribute rather than flattened
export const pageLinksSchema = z
  .object({
    byAttribute: z
      .record(z.string(), z.array(z.string()))
      .describe('Keyed by attribute ID: the chosen value IDs of a selection, or the linked entity IDs of a relationship in order'),
  })
  .describe("A CMS page's selection and relationship attribute values")

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

// ---------- Tags: slugs from the shared library ----------

export const pageTagsSchema = z
  .object({ tags: z.array(z.string()).describe('Slugs of tags from the shared tag library') })
  .describe("A CMS page's tags")

export type PageTagsValues = z.infer<typeof pageTagsSchema>

export const PAGE_TAGS_LABELS: Record<keyof PageTagsValues, string> = { tags: 'Tags' }

export function pageTagsDiff(tagName: (slug: string) => string) {
  const label = (slugs: string[]) => slugs.map(tagName).sort().join(', ') || 'none'
  return (original: PageTagsValues, draft: PageTagsValues): SectionDiffEntry[] =>
    [...original.tags].sort().join(' ') === [...draft.tags].sort().join(' ')
      ? []
      : [{ kind: 'change', label: PAGE_TAGS_LABELS.tags, before: label(original.tags), after: label(draft.tags) }]
}

// ---------- Channels: one listing per channel ----------

// `listed` - a listing row exists; without one the page falls back to its global publish state
export type ListingDraft = { listed: boolean; isPublished: boolean; isHomepage: boolean; seoNoindex: boolean }
export const EMPTY_LISTING: ListingDraft = { listed: false, isPublished: false, isHomepage: false, seoNoindex: false }

export const pageChannelsSchema = z
  .object({
    listings: z
      .record(
        z.string(),
        z.object({
          listed: z.boolean().describe('The page has a listing in this channel - without one it follows its global publish state'),
          isPublished: z.boolean().describe('Published in this channel'),
          isHomepage: z.boolean().describe("The page is this channel's homepage"),
          seoNoindex: z.boolean().describe('Ask search engines not to index the page in this channel'),
        }),
      )
      .describe('Per-channel listing, keyed by channel ID'),
  })
  .describe("A CMS page's listing in each channel")

export type PageChannelsValues = z.infer<typeof pageChannelsSchema>

export const PAGE_CHANNELS_LABELS: Record<keyof PageChannelsValues, string> = { listings: 'Channels' }

export function toPageChannelsValues(listings: { channelId: string; isPublished: boolean; isHomepage: boolean; seoNoindex: boolean }[]): PageChannelsValues {
  return {
    listings: Object.fromEntries(
      listings.map((l) => [l.channelId, { listed: true, isPublished: l.isPublished, isHomepage: l.isHomepage, seoNoindex: l.seoNoindex }]),
    ),
  }
}

export const listingOf = (v: PageChannelsValues, channelId: string): ListingDraft => v.listings[channelId] ?? EMPTY_LISTING

export function changedChannels(before: PageChannelsValues, after: PageChannelsValues): string[] {
  const ids = new Set([...Object.keys(before.listings), ...Object.keys(after.listings)])
  return [...ids].filter((id) => JSON.stringify(listingOf(before, id)) !== JSON.stringify(listingOf(after, id)))
}

export function listingStateLabel(s: ListingDraft): string {
  if (!s.listed) return 'Not listed'
  return [s.isPublished ? 'Published' : 'Unpublished', ...(s.isHomepage ? ['homepage'] : [])].join(', ')
}

export function pageChannelsDiff(channelName: (id: string) => string) {
  return (original: PageChannelsValues, draft: PageChannelsValues): SectionDiffEntry[] =>
    changedChannels(original, draft).map((id) => ({
      kind: 'change',
      label: channelName(id),
      before: listingStateLabel(listingOf(original, id)),
      after: listingStateLabel(listingOf(draft, id)),
    }))
}

// ---------- Content: the edited locale's title, SEO, translatable fields and block layout ----------

export const pageContentSchema = z
  .object({
    // The service refuses a content row without one
    title: z.string().trim().min(1, 'Title is required').describe('Title in the edited locale'),
    metaTitle: z.string().describe("SEO title in the edited locale - empty string uses the page type's default"),
    metaDescription: z.string().describe('SEO description in the edited locale - empty string for none'),
    data: z
      .record(z.string(), z.unknown())
      .describe("Values of the page type's translatable attributes in the edited locale, keyed by attribute slug"),
    // Opaque to the form - the block tree's recursive type would blow up TanStack's deep field paths
    blocks: z.array(z.unknown()).describe('The page layout in the edited locale, as an ordered list of blocks'),
  })
  .describe("A CMS page's title, SEO copy, translatable field values and block layout in the edited locale")

export type PageContentValues = z.infer<typeof pageContentSchema>

export const PAGE_CONTENT_LABELS: Record<keyof PageContentValues, string> = {
  title: 'Title',
  metaTitle: 'Meta title',
  metaDescription: 'Meta description',
  data: 'Fields',
  blocks: 'Page layout',
}

type ExistingContent = { locale: string; title: string; body?: unknown; metaTitle?: string | null; metaDescription?: string | null; data?: unknown }

// A freeform (Lexical) body parses to no blocks - the builder starts empty
export function toPageContentValues(allContent: ExistingContent[], locale: string, parseBlocks: (body: unknown) => unknown[]): PageContentValues {
  const c = allContent.find((x) => x.locale === locale)
  return {
    title: c?.title ?? '',
    metaTitle: c?.metaTitle ?? '',
    metaDescription: c?.metaDescription ?? '',
    data: (c?.data as Record<string, FieldValue> | null) ?? {},
    blocks: parseBlocks(c?.body),
  }
}

// Blocks carry ids, so the summary says what happened to them - a props-only edit still reads as a
// change ("3 blocks -> 3 blocks (1 edited)") rather than an identical before/after the registry drops
function blocksEntry(before: unknown[], after: unknown[]): SectionDiffEntry | null {
  if (JSON.stringify(before) === JSON.stringify(after)) return null
  const idOf = (b: unknown) => (b as { id: string }).id
  const was = new Map(before.map((b) => [idOf(b), JSON.stringify(b)]))
  const now = new Set(after.map(idOf))
  const addedN = after.filter((b) => !was.has(idOf(b))).length
  const removedN = before.filter((b) => !now.has(idOf(b))).length
  const editedN = after.filter((b) => was.has(idOf(b)) && was.get(idOf(b)) !== JSON.stringify(b)).length
  const parts = [addedN && `${addedN} added`, removedN && `${removedN} removed`, editedN && `${editedN} edited`].filter(Boolean)
  const count = (n: number) => `${n} block${n === 1 ? '' : 's'}`
  return { kind: 'change', label: PAGE_CONTENT_LABELS.blocks, before: count(before.length), after: `${count(after.length)} (${parts.length ? parts.join(', ') : 'reordered'})` }
}

export function pageContentDiff(translatableAttrs: PageTypeAttr[]) {
  return (original: PageContentValues, draft: PageContentValues): SectionDiffEntry[] => [
    ...fieldDiff(original, draft, [
      { label: PAGE_CONTENT_LABELS.title, value: (v) => v.title },
      { label: PAGE_CONTENT_LABELS.metaTitle, value: (v) => v.metaTitle },
      { label: PAGE_CONTENT_LABELS.metaDescription, value: (v) => v.metaDescription },
    ]),
    ...[blocksEntry(original.blocks, draft.blocks)].filter((e) => e !== null),
    ...dynamicFieldDiff(translatableAttrs, original.data as Record<string, FieldValue>, draft.data as Record<string, FieldValue>),
  ]
}
