import { z } from 'zod'

// New menu - just the identity. Items are added on the menu's own page afterwards

export const newMenuSchema = z.object({
  name: z.string().trim().min(1, 'Name is required'),
  // The backend's menu slug rule (internal/menu validateSlug)
  slug: z
    .string()
    .trim()
    .min(1, 'Slug is required')
    .regex(/^[a-z0-9_-]+$/, 'Lowercase letters, digits, hyphens and underscores only'),
})

export type NewMenuValues = z.infer<typeof newMenuSchema>

export const emptyNewMenu: NewMenuValues = { name: '', slug: '' }

export const NEW_MENU_LABELS: Record<keyof NewMenuValues, string> = { name: 'Name', slug: 'Slug' }

export function toNewMenuInput(v: NewMenuValues) {
  return { slug: v.slug.trim(), name: v.name.trim() }
}
