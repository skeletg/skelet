import { z } from 'zod'
import { numberText } from '@/shared/schemas/fields'
import type { SectionDiffEntry } from '@/shared/saving/saveRegistry'
import { added, changed, fieldDiff, removed } from '@/shared/saving/diff'
import { richTextToPlainText, type RichTextValue } from '@/features/catalog/LexicalRichTextEditor'

// Blog post page - THE field list (docs/conventions.md, "Record forms"): the post's own settings, the
// edited locale's content, and its categories and tags. Publishing, scheduling, archiving and the
// per-store listings are actions, not fields

const postShape = z.object({
  slug: z
    .string()
    .trim()
    .min(1, 'Slug is required')
    // The backend's content slug rule (internal/content.ValidateSlug)
    .regex(/^[a-z0-9._-]+$/, 'Lowercase letters, numbers, dots, underscores and hyphens only'),
  seoNoindex: z.boolean(),
  // As typed - blank means "let the storefront estimate it"
  readingTime: numberText({ optional: true, int: true, min: 1 }, 'Whole minutes, one or more'),
  title: z.string(),
  // Opaque rich-text JSON to the form - its recursive type would blow up TanStack's deep field paths
  body: z.unknown(),
  excerpt: z.unknown(),
  metaTitle: z.string(),
  metaDescription: z.string(),
  categoryIds: z.array(z.string()),
  tagIds: z.array(z.string()),
})

export type PostValues = z.infer<typeof postShape>

// Built against the stored post: the content row is written only with a title, so the title is
// required once any content field changed - an untouched, never-translated locale stays blank
export function postSchema(baseline: PostValues) {
  return postShape.superRefine((v, ctx) => {
    if (postChanges(baseline, v).content !== null && !v.title.trim()) {
      ctx.addIssue({ code: 'custom', path: ['title'], message: 'A translation needs a title' })
    }
  })
}

export const POST_LABELS: Record<keyof PostValues, string> = {
  slug: 'Slug',
  seoNoindex: 'Noindex',
  readingTime: 'Reading time',
  title: 'Title',
  body: 'Body',
  excerpt: 'Excerpt',
  metaTitle: 'Meta title',
  metaDescription: 'Meta description',
  categoryIds: 'Categories',
  tagIds: 'Tags',
}

type ExistingPost = {
  slug: string
  seoNoindex: boolean
  readingTimeMinutes?: number | null
  allContent: { locale: string; title: string; body?: unknown; excerpt?: unknown; metaTitle?: string | null; metaDescription?: string | null }[]
  categories: { id: string }[]
  tags: { id: string }[]
}

export function toPostValues(post: ExistingPost, locale: string): PostValues {
  const content = post.allContent.find((c) => c.locale === locale)
  return {
    slug: post.slug,
    seoNoindex: post.seoNoindex,
    readingTime: post.readingTimeMinutes ? String(post.readingTimeMinutes) : '',
    title: content?.title ?? '',
    body: content?.body ?? null,
    excerpt: content?.excerpt ?? null,
    metaTitle: content?.metaTitle ?? '',
    metaDescription: content?.metaDescription ?? '',
    categoryIds: post.categories.map((c) => c.id),
    tagIds: post.tags.map((t) => t.id),
  }
}

const sameJson = (a: unknown, b: unknown) => JSON.stringify(a ?? null) === JSON.stringify(b ?? null)
const sameSet = (a: string[], b: string[]) => a.length === b.length && a.every((x) => b.includes(x))

// The post's writes, each only when its part changed
export function postChanges(before: PostValues, after: PostValues) {
  const settings = before.slug !== after.slug || before.seoNoindex !== after.seoNoindex || before.readingTime !== after.readingTime
  const content =
    before.title !== after.title ||
    before.metaTitle !== after.metaTitle ||
    before.metaDescription !== after.metaDescription ||
    !sameJson(before.body, after.body) ||
    !sameJson(before.excerpt, after.excerpt)
  return {
    settings: settings
      ? {
          // The service turns a changed slug into a redirect - send it only when it changed
          slug: after.slug !== before.slug ? after.slug.trim() : null,
          seoNoindex: after.seoNoindex,
          readingTimeMinutes: after.readingTime.trim() ? Number(after.readingTime) : null,
        }
      : null,
    content: content
      ? {
          title: after.title.trim(),
          body: after.body as RichTextValue,
          excerpt: after.excerpt as RichTextValue,
          metaTitle: after.metaTitle,
          metaDescription: after.metaDescription,
        }
      : null,
    categoryIds: sameSet(before.categoryIds, after.categoryIds) ? null : after.categoryIds,
    tagIds: sameSet(before.tagIds, after.tagIds) ? null : after.tagIds,
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

export function postDiff(names: { category: (id: string) => string; tag: (id: string) => string }) {
  const setDiff = (label: string, name: (id: string) => string, before: string[], after: string[]) => [
    ...after.filter((id) => !before.includes(id)).map((id) => added(`${label} · ${name(id)}`)),
    ...before.filter((id) => !after.includes(id)).map((id) => removed(`${label} · ${name(id)}`)),
  ]
  return (original: PostValues, draft: PostValues): SectionDiffEntry[] => [
    ...fieldDiff(original, draft, [
      { label: POST_LABELS.slug, value: (v) => v.slug },
      { label: POST_LABELS.seoNoindex, value: (v) => v.seoNoindex },
      { label: POST_LABELS.readingTime, value: (v) => v.readingTime },
      { label: POST_LABELS.title, value: (v) => v.title },
      { label: POST_LABELS.metaTitle, value: (v) => v.metaTitle },
      { label: POST_LABELS.metaDescription, value: (v) => clip(v.metaDescription) },
    ]),
    ...[richTextEntry(POST_LABELS.excerpt, original.excerpt, draft.excerpt), richTextEntry(POST_LABELS.body, original.body, draft.body)].filter(
      (e) => e !== null,
    ),
    ...setDiff('Category', names.category, original.categoryIds, draft.categoryIds),
    ...setDiff('Tag', names.tag, original.tagIds, draft.tagIds),
  ]
}
