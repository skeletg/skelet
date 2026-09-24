import { z } from 'zod'
import type { RichTextValue } from '../LexicalRichTextEditor'

// New collection - the record and its first translation. Cover image, product membership,
// redirects and the rest are managed on the collection's own page afterwards

// `defaultLocale` stands in for an unpicked locale - the system default when this admin can see it,
// else their first allowed one
export function newCollectionSchema(defaultLocale: string) {
  return z.object({
    name: z.string().trim().min(1, 'Name is required'),
    slug: z
      .string()
      .trim()
      .min(1, 'Slug is required')
      .regex(/^[a-z0-9_-]+$/, 'Lowercase letters, numbers, underscores and hyphens only'),
    locale: z.string().refine((v) => (v || defaultLocale) !== '', 'Pick a locale'),
    isPublished: z.boolean(),
    seoNoindex: z.boolean(),
    // Opaque rich-text JSON to the form - its recursive type would blow up TanStack's deep field paths
    description: z.unknown(),
    // The editor's plain-text read of the description - blank means "no description"
    descriptionText: z.string(),
    metaTitle: z.string(),
    metaDescription: z.string(),
    metaKeywords: z.string(),
  })
}

export type NewCollectionValues = z.infer<ReturnType<typeof newCollectionSchema>>

export const emptyNewCollection: NewCollectionValues = {
  name: '',
  slug: '',
  locale: '',
  isPublished: false,
  seoNoindex: false,
  description: null,
  descriptionText: '',
  metaTitle: '',
  metaDescription: '',
  metaKeywords: '',
}

export function toNewCollectionInput(v: NewCollectionValues, defaultLocale: string) {
  return {
    slug: v.slug.trim(),
    locale: v.locale || defaultLocale,
    name: v.name.trim(),
    isPublished: v.isPublished,
    seoNoindex: v.seoNoindex,
    description: v.descriptionText.trim() ? (v.description as RichTextValue) : undefined,
    metaTitle: v.metaTitle.trim() || undefined,
    metaDescription: v.metaDescription.trim() || undefined,
    metaKeywords: v.metaKeywords.trim() || undefined,
  }
}
