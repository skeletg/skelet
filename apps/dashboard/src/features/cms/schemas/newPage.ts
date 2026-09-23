import { z } from 'zod'

// New CMS page - its page type decides the editor's custom fields, filled in after creation

export const newPageSchema = z.object({
  pageTypeId: z.string().min(1, 'Pick a page type'),
  slug: z.string().trim().min(1, 'Slug is required'),
  title: z.string().trim().min(1, 'Title is required'),
  locale: z.string().min(1, 'Pick the initial locale'),
})

export type NewPageValues = z.infer<typeof newPageSchema>

export function toNewPageInput(v: NewPageValues) {
  return { pageTypeId: v.pageTypeId, slug: v.slug.trim(), locale: v.locale, title: v.title.trim() }
}
