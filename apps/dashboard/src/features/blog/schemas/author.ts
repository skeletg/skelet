import { z } from 'zod'
import { optionalEmail } from '@/shared/schemas/fields'
import type { SectionDiffEntry } from '@/shared/saving/saveRegistry'
import { fieldDiff } from '@/shared/saving/diff'
import type { BlogAuthorRowFragment } from '../Blog.gql'

// Blog author form - THE field list (docs/conventions.md, "Record forms"). The bio is per locale;
// slug, display name and email are the author's own

export const authorSchema = z
  .object({
    slug: z.string().trim().min(1, 'Slug is required').describe("URL slug of the author's page"),
    displayName: z.string().trim().min(1, 'Display name is required').describe('Name shown on the posts they wrote'),
    email: optionalEmail().describe("The author's contact email, or an empty string for none"),
    bio: z.string().describe("Plain-text bio in the edited locale - empty string for none"),
  })
  .describe('A blog author')

export type AuthorValues = z.infer<typeof authorSchema>

export const AUTHOR_LABELS: Record<keyof AuthorValues, string> = {
  slug: 'Slug',
  displayName: 'Display name',
  email: 'Email',
  bio: 'Bio',
}

// A lexical-shaped rich-text node carrying the bio as one plain-text paragraph - the editor here
// only ever writes plain text
export function bioFromText(text: string) {
  return text ? { root: { children: [{ type: 'paragraph', children: [{ type: 'text', text }] }] } } : null
}

// Whatever plain text a stored lexical bio holds, for editing
export function bioToText(bio: unknown): string {
  const children = (bio as { root?: { children?: unknown } } | null)?.root?.children
  if (!Array.isArray(children)) return ''
  const parts: string[] = []
  for (const block of children) {
    for (const node of (block as { children?: { text?: unknown }[] })?.children ?? []) {
      if (typeof node?.text === 'string') parts.push(node.text)
    }
  }
  return parts.join('\n')
}

export function toAuthorValues(author?: BlogAuthorRowFragment): AuthorValues {
  return {
    slug: author?.slug ?? '',
    displayName: author?.displayName ?? '',
    email: author?.email ?? '',
    bio: bioToText(author?.content?.bio ?? author?.allContent[0]?.bio),
  }
}

export function authorDiff(original: AuthorValues, draft: AuthorValues): SectionDiffEntry[] {
  return fieldDiff(
    original,
    draft,
    (Object.keys(AUTHOR_LABELS) as (keyof AuthorValues)[]).map((k) => ({ label: AUTHOR_LABELS[k], value: (o) => o[k] })),
  )
}
