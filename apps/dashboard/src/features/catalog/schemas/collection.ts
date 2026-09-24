import { z } from 'zod'
import type { SectionDiffEntry } from '@/shared/saving/saveRegistry'
import { changed, fieldDiff } from '@/shared/saving/diff'
import { cleanMetadata, metadataDiff, metadataSchema, toMetadataValues, type MetadataEntry } from '@/shared/schemas/metadata'
import { richTextToPlainText, type RichTextValue } from '../LexicalRichTextEditor'

// Collection page - THE field list (docs/conventions.md, "Record forms"): the record's slug and
// flags, the edited locale's translation (name, description, SEO) and both metadata lists. The cover
// image (a file) and the storefront filter order stage as their own sections; products write at once

// Value without a key, or a key used twice - the backend keys metadata by name
function metadataIssues(rows: MetadataEntry[], path: string, label: string, ctx: z.RefinementCtx) {
  if (rows.some((r) => !r.key.trim() && r.value.trim())) {
    ctx.addIssue({ code: 'custom', path: [path], message: `${label} entries with a value need a key` })
    return
  }
  const keys = cleanMetadata(rows).map((r) => r.key)
  const dupe = keys.find((k, i) => keys.indexOf(k) !== i)
  if (dupe) ctx.addIssue({ code: 'custom', path: [path], message: `Duplicate ${label.toLowerCase()} key: "${dupe}"` })
}

const collectionShape = z.object({
  slug: z
    .string()
    .trim()
    .min(1, 'Slug is required')
    // The backend's content slug rule (internal/content.ValidateSlug)
    .regex(/^[a-z0-9._-]+$/, 'Lowercase letters, numbers, dots, underscores and hyphens only'),
  isPublished: z.boolean(),
  seoNoindex: z.boolean(),
  name: z.string(),
  // Opaque rich-text JSON to the form - its recursive type would blow up TanStack's deep field paths
  description: z.unknown(),
  // The editor's plain-text read of the description - blank means "no description"
  descriptionText: z.string(),
  metaTitle: z.string(),
  metaDescription: z.string(),
  metaKeywords: z.string(),
  metadata: metadataSchema,
  privateMetadata: metadataSchema,
})

export type CollectionValues = z.infer<typeof collectionShape>

// Built against the stored record: the translation row can only be written with a name, so a name is
// required once any of its fields changed - an untouched, never-translated locale stays blank
export function collectionSchema(baseline: CollectionValues) {
  return collectionShape.superRefine((v, ctx) => {
    if (collectionChanges(baseline, v).content !== null && !v.name.trim()) {
      ctx.addIssue({ code: 'custom', path: ['name'], message: 'A translation needs a name' })
    }
    metadataIssues(v.metadata, 'metadata', 'Metadata', ctx)
    metadataIssues(v.privateMetadata, 'privateMetadata', 'Private metadata', ctx)
  })
}

export const COLLECTION_LABELS: Record<keyof CollectionValues, string> = {
  slug: 'Slug',
  isPublished: 'Published',
  seoNoindex: 'Noindex',
  name: 'Name',
  description: 'Description',
  descriptionText: 'Description',
  metaTitle: 'Meta title',
  metaDescription: 'Meta description',
  metaKeywords: 'Meta keywords',
  metadata: 'Metadata',
  privateMetadata: 'Private metadata',
}

type ExistingCollection = {
  slug: string
  isPublished: boolean
  seoNoindex: boolean
  name?: string | null
  metaTitle?: string | null
  metaDescription?: string | null
  metaKeywords?: string | null
  content: { locale: string; name: string; description?: unknown; metaTitle?: string | null; metaDescription?: string | null; metaKeywords?: string | null }[]
  metadata: MetadataEntry[]
  privateMetadata: MetadataEntry[]
}

export function toCollectionValues(c: ExistingCollection, locale: string): CollectionValues {
  const content = c.content.find((e) => e.locale === locale)
  return {
    slug: c.slug,
    isPublished: c.isPublished,
    seoNoindex: c.seoNoindex,
    name: content?.name ?? c.name ?? '',
    description: content?.description ?? null,
    descriptionText: richTextToPlainText(content?.description as RichTextValue),
    metaTitle: content?.metaTitle ?? c.metaTitle ?? '',
    metaDescription: content?.metaDescription ?? c.metaDescription ?? '',
    metaKeywords: content?.metaKeywords ?? c.metaKeywords ?? '',
    metadata: toMetadataValues(c.metadata),
    privateMetadata: toMetadataValues(c.privateMetadata),
  }
}

const CONTENT = ['name', 'descriptionText', 'metaTitle', 'metaDescription', 'metaKeywords'] as const

// The writes the edits need: the record's own update (slug, flags, metadata) and the translation row,
// which the upsert replaces whole - so any content edit sends every content field
export function collectionChanges(before: CollectionValues, after: CollectionValues) {
  const metadataChanged =
    metadataDiff(before.metadata, after.metadata).length > 0 || metadataDiff(before.privateMetadata, after.privateMetadata).length > 0
  const basicsChanged = before.slug !== after.slug || before.isPublished !== after.isPublished || before.seoNoindex !== after.seoNoindex
  const contentChanged =
    CONTENT.some((k) => before[k] !== after[k]) || JSON.stringify(before.description ?? null) !== JSON.stringify(after.description ?? null)
  return {
    update:
      basicsChanged || metadataChanged
        ? {
            ...(basicsChanged ? { slug: after.slug.trim(), isPublished: after.isPublished, seoNoindex: after.seoNoindex } : {}),
            ...(metadataChanged ? { metadata: cleanMetadata(after.metadata), privateMetadata: cleanMetadata(after.privateMetadata) } : {}),
          }
        : null,
    content: contentChanged
      ? {
          name: after.name.trim(),
          description: after.descriptionText.trim() ? (after.description as RichTextValue) : undefined,
          metaTitle: after.metaTitle.trim() || undefined,
          metaDescription: after.metaDescription.trim() || undefined,
          metaKeywords: after.metaKeywords.trim() || undefined,
        }
      : null,
  }
}

const clip = (s: string, max = 80) => (s.length > max ? `${s.slice(0, max - 1)}…` : s)

export function collectionDiff(original: CollectionValues, draft: CollectionValues): SectionDiffEntry[] {
  // Rich text shows its plain text; a formatting-only edit (same text, different JSON) still counts
  const formatting =
    original.descriptionText === draft.descriptionText &&
    JSON.stringify(original.description ?? null) !== JSON.stringify(draft.description ?? null)
      ? changed(COLLECTION_LABELS.description, 'previous formatting', 'updated formatting')
      : null
  return [
    ...fieldDiff(original, draft, [
      { label: COLLECTION_LABELS.slug, value: (v) => v.slug },
      { label: COLLECTION_LABELS.isPublished, value: (v) => (v.isPublished ? 'Published' : 'Draft') },
      { label: COLLECTION_LABELS.seoNoindex, value: (v) => v.seoNoindex },
      { label: COLLECTION_LABELS.name, value: (v) => v.name },
      { label: COLLECTION_LABELS.descriptionText, value: (v) => clip(v.descriptionText) },
      { label: COLLECTION_LABELS.metaTitle, value: (v) => v.metaTitle },
      { label: COLLECTION_LABELS.metaDescription, value: (v) => clip(v.metaDescription) },
      { label: COLLECTION_LABELS.metaKeywords, value: (v) => v.metaKeywords },
    ]),
    ...(formatting ? [formatting] : []),
    ...metadataDiff(original.metadata, draft.metadata),
    ...metadataDiff(original.privateMetadata, draft.privateMetadata, 'Private metadata'),
  ]
}
