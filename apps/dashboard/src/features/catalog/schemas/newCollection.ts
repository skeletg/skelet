import { z } from 'zod'
import { contentSlug } from '@/shared/schemas/fields'
import type { RichTextValue } from '../LexicalRichTextEditor'

// New collection - the record and its first translation. Cover image, product membership,
// redirects and the rest are managed on the collection's own page afterwards

// `defaultLocale` stands in for an unpicked locale - the system default when this admin can see it,
// else their first allowed one
export function newCollectionSchema(defaultLocale: string) {
  return z
    .object({
      name: z.string().trim().min(1, 'Name is required').describe('Name of the collection in the initial locale'),
      slug: contentSlug().describe('URL slug of the collection - lowercase letters, numbers, dots, underscores and hyphens'),
      locale: z
        .string()
        .refine((v) => (v || defaultLocale) !== '', 'Pick a locale')
        .describe(`Locale code the first translation is written in - empty string uses ${defaultLocale || 'the default locale'}`),
      isPublished: z.boolean().describe('Whether the collection is visible on the storefront'),
      seoNoindex: z.boolean().describe('Ask search engines not to index the collection page'),
      // Opaque rich-text JSON to the form - its recursive type would blow up TanStack's deep field paths
      description: z.unknown().describe('Description, as Lexical rich-text JSON, or null'),
      descriptionText: z.string().describe('Plain-text read of the description - empty string means no description'),
      metaTitle: z.string().describe('SEO title - empty string for none'),
      metaDescription: z.string().describe('SEO description - empty string for none'),
      metaKeywords: z.string().describe('SEO keywords - empty string for none'),
    })
    .describe('A new product collection and its first translation')
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
