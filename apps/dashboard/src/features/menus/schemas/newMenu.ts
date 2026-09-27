import { z } from 'zod'
import { handleSlug } from '@/shared/schemas/fields'

// New menu - just the identity. Items are added on the menu's own page afterwards

export const newMenuSchema = z
  .object({
    name: z.string().trim().min(1, 'Name is required').describe('Display name of the menu'),
    slug: handleSlug().describe('Handle the storefront fetches the menu by - lowercase letters, numbers, underscores and hyphens'),
  })
  .describe('A new navigation menu')

export type NewMenuValues = z.infer<typeof newMenuSchema>

export const emptyNewMenu: NewMenuValues = { name: '', slug: '' }

export const NEW_MENU_LABELS: Record<keyof NewMenuValues, string> = { name: 'Name', slug: 'Slug' }

export function toNewMenuInput(v: NewMenuValues) {
  return { slug: v.slug.trim(), name: v.name.trim() }
}
