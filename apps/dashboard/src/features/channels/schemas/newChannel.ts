import { z } from 'zod'

// New channel dialog - just the identity. Branding, contact, locales and the rest are filled in
// under Settings once the channel exists

export const newChannelSchema = z.object({
  // Stable identifier for tooling and the X-Store-Slug header - fixed after create
  slug: z
    .string()
    .trim()
    .min(1, 'Slug is required')
    .regex(/^[a-z0-9_-]+$/, 'Lowercase letters, digits, dashes, underscores only'),
  name: z.string().trim().min(1, 'Name is required'),
  currencyCode: z.string().min(1, 'Currency is required'),
  defaultLocale: z.string().min(1, 'Default locale is required'),
})

export type NewChannelValues = z.infer<typeof newChannelSchema>

export const emptyNewChannel: NewChannelValues = { slug: '', name: '', currencyCode: '', defaultLocale: '' }

export function toNewChannelInput(v: NewChannelValues) {
  return { slug: v.slug.trim(), name: v.name.trim(), currencyCode: v.currencyCode.toUpperCase(), defaultLocale: v.defaultLocale }
}
