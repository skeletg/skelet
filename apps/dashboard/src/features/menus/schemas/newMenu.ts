import { z } from 'zod'
import { handleSlug } from '@/shared/schemas/fields'

// New menu - just the identity. Items are added on the menu's own page afterwards

export const newMenuSchema = z.object({
  name: z.string().trim().min(1, 'Name is required'),
  slug: handleSlug(),
})

export type NewMenuValues = z.infer<typeof newMenuSchema>

export const emptyNewMenu: NewMenuValues = { name: '', slug: '' }

export const NEW_MENU_LABELS: Record<keyof NewMenuValues, string> = { name: 'Name', slug: 'Slug' }

export function toNewMenuInput(v: NewMenuValues) {
  return { slug: v.slug.trim(), name: v.name.trim() }
}
