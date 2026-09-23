import { z } from 'zod'

// New CMS page type - a page template. The name and description are seeded for the locale being
// edited; attributes are assigned on the type's own page afterwards

export const newPageTypeSchema = z.object({
  slug: z.string().trim().min(1, 'Slug is required'),
  name: z.string().trim().min(1, 'Name is required'),
  description: z.string(),
  icon: z.string(),
})

export type NewPageTypeValues = z.infer<typeof newPageTypeSchema>

export const emptyNewPageType: NewPageTypeValues = { slug: '', name: '', description: '', icon: '' }

export function toNewPageTypeInput(v: NewPageTypeValues, locale: string) {
  return {
    slug: v.slug.trim(),
    icon: v.icon.trim() || null,
    isSystem: false,
    position: 0,
    locale,
    name: v.name.trim(),
    description: v.description.trim() || null,
  }
}
