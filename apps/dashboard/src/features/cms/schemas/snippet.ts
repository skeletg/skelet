import { z } from 'zod'
import { contentSlug } from '@/shared/schemas/fields'
import { parseDocument, serializeDocument, type CmsBlock } from '../page-blocks'
import type { CmsSnippetsQuery } from '../Cms.gql'

// Reusable section (snippet) - THE field list (docs/conventions.md, "Record forms"). The slug is how
// pages reference the section, so it's fixed once created

type Snippet = CmsSnippetsQuery['cmsSnippets'][number]

export const snippetSchema = z.object({
  name: z.string().trim().min(1, 'Name is required'),
  slug: contentSlug(),
  // Opaque to the form - the block tree's recursive type would blow up TanStack's deep field paths
  blocks: z.array(z.unknown()),
})

export type SnippetValues = z.infer<typeof snippetSchema>

export function toSnippetValues(snippet: Snippet | null): SnippetValues {
  return {
    name: snippet?.name ?? '',
    slug: snippet?.slug ?? '',
    blocks: parseDocument(snippet?.blocks)?.blocks ?? [],
  }
}

export function toSnippetBlocks(v: SnippetValues) {
  return serializeDocument(v.blocks as CmsBlock[])
}
