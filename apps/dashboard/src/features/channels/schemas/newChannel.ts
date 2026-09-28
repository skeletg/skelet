import { z } from 'zod'
import { handleSlug } from '@/shared/schemas/fields'

// New channel dialog - just the identity. Branding, contact, locales and the rest are filled in
// under Settings once the channel exists

export const newChannelSchema = z
  .object({
    slug: handleSlug().describe(
      'Stable identifier for tooling and the X-Store-Slug header, never changed - lowercase letters, numbers, underscores and hyphens',
    ),
    name: z.string().trim().min(1, 'Name is required').describe('Display name of the channel (store)'),
    currencyCode: z.string().min(1, 'Currency is required').describe('ISO 4217 code of the currency the channel sells in, such as USD'),
    defaultLocale: z.string().min(1, 'Default locale is required').describe("Code of the channel's default locale, such as en-US"),
  })
  .describe('A new sales channel (store)')

export type NewChannelValues = z.infer<typeof newChannelSchema>

export const emptyNewChannel: NewChannelValues = { slug: '', name: '', currencyCode: '', defaultLocale: '' }

export function toNewChannelInput(v: NewChannelValues) {
  return { slug: v.slug.trim(), name: v.name.trim(), currencyCode: v.currencyCode.toUpperCase(), defaultLocale: v.defaultLocale }
}
