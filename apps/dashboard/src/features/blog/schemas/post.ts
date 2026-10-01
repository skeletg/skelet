import { z } from 'zod'
import { contentSlug, numberText } from '@/shared/schemas/fields'
import type { SectionDiffEntry } from '@/shared/saving/saveRegistry'
import { added, clip, fieldDiff, removed, sameJson, sameSet } from '@/shared/saving/diff'
import type { RichTextValue } from '@/features/catalog/LexicalRichTextEditor'
import { richTextEntry } from '@/features/catalog/schemas/rich-text-diff'

// Blog post page - THE field list (docs/conventions.md, "Record forms"): the post's own settings, the
// edited locale's content, and its categories and tags. Publishing, scheduling, archiving and the
// per-store listings are actions, not fields

const postShape = z
  .object({
    slug: contentSlug().describe('URL slug of the post - lowercase letters, numbers, dots, underscores and hyphens'),
    seoNoindex: z.boolean().describe('Ask search engines not to index the post'),
    readingTime: numberText({ optional: true, int: true, min: 1 }, 'Whole minutes, one or more').describe(
      'Reading time in whole minutes, as text - empty string lets the storefront estimate it',
    ),
    title: z.string().describe("Title in the edited locale - required once any of that locale's content is set"),
    // Opaque rich-text JSON to the form - its recursive type would blow up TanStack's deep field paths
    body: z.unknown().describe('Body in the edited locale, as Lexical rich-text JSON, or null'),
    excerpt: z.unknown().describe('Teaser in the edited locale, as Lexical rich-text JSON, or null'),
    metaTitle: z.string().describe('SEO title in the edited locale - empty string for none'),
    metaDescription: z.string().describe('SEO description in the edited locale - empty string for none'),
    categoryIds: z.array(z.string()).describe('IDs of the blog categories the post is filed under'),
    tagIds: z.array(z.string()).describe('IDs of the blog tags on the post'),
  })
  .describe("A blog post's settings, its content in the edited locale, and its categories and tags")

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
