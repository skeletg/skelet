import { z } from 'zod'

// New CMS page - its page type decides the editor's custom fields, filled in after creation

export const newPageSchema = z
  .object({
    pageTypeId: z.string().min(1, 'Pick a page type').describe("ID of the page type - decides the page's custom fields"),
    slug: z.string().trim().min(1, 'Slug is required').describe('URL slug of the page'),
    title: z.string().trim().min(1, 'Title is required').describe('Title in the initial locale'),
    locale: z.string().min(1, 'Pick the initial locale').describe('Locale code the first translation is written in, such as en-US'),
  })
  .describe('A new CMS page')

export type NewPageValues = z.infer<typeof newPageSchema>

export function toNewPageInput(v: NewPageValues) {
  return { pageTypeId: v.pageTypeId, slug: v.slug.trim(), locale: v.locale, title: v.title.trim() }
}
