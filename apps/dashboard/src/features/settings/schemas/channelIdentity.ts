import { z } from 'zod'
import { currencyCode, optionalAbsoluteUrl } from '@/shared/schemas/fields'
import type { SectionDiffEntry } from '@/shared/saving/saveRegistry'
import { fieldDiff } from '@/shared/saving/diff'
import type { ChannelQuery } from '@/features/channels/Channels.gql'

// A channel's identity on the Settings page - THE field list (docs/conventions.md, "Record forms").
// Slug is fixed after create and the default locale is edited beside the supported locales, so
// neither is here

export const channelIdentitySchema = z
  .object({
    name: z.string().trim().min(1, 'Name is required').describe('Display name of the channel (store)'),
    currencyCode: currencyCode('Pick a currency').describe('Currency the channel sells in, as an upper-case ISO 4217 code such as USD'),
    publicUrl: optionalAbsoluteUrl().describe(
      "Storefront base URL that password-reset and verification email links are built on - empty string for none",
    ),
    isActive: z.boolean().describe('Whether the channel is live'),
  })
  .describe("A channel's name, currency, public URL and active flag")

export type ChannelIdentityValues = z.infer<typeof channelIdentitySchema>

export const CHANNEL_IDENTITY_LABELS: Record<keyof ChannelIdentityValues, string> = {
  name: 'Name',
  currencyCode: 'Currency',
  publicUrl: 'Public URL',
  isActive: 'Active',
}

type Channel = NonNullable<ChannelQuery['channel']>

export function toChannelIdentityValues(channel: Channel): ChannelIdentityValues {
  return {
    name: channel.name,
    currencyCode: channel.currencyCode,
    publicUrl: channel.publicUrl ?? '',
    isActive: channel.isActive,
  }
}

// The update input is the whole channel - the fields this form doesn't own go back unchanged
export function toChannelIdentityInput(v: ChannelIdentityValues, channel: Channel) {
  return {
    id: channel.id,
    name: v.name.trim(),
    defaultLocale: channel.defaultLocale,
    isActive: v.isActive,
    currencyCode: v.currencyCode,
    publicUrl: v.publicUrl.trim() || null,
    metadata: channel.metadata,
  }
}

export function channelIdentityDiff(original: ChannelIdentityValues, draft: ChannelIdentityValues): SectionDiffEntry[] {
  return fieldDiff(
    original,
    draft,
    (Object.keys(CHANNEL_IDENTITY_LABELS) as (keyof ChannelIdentityValues)[]).map((k) => ({
      label: CHANNEL_IDENTITY_LABELS[k],
      value: (o) => o[k],
    })),
  )
}

// The backend's currency guards name a code, not a reason - say what the admin has to fix
export function friendlyCurrencyError(message: string): string {
  if (message.includes('STORE_CURRENCY_BLOCKED')) {
    return 'Some listings inherit price on this channel. Set explicit prices first, then change currency.'
  }
  if (message.includes('GLOBAL_CURRENCY_BLOCKED')) {
    return 'Some inheriting listings would become incoherent under the new global currency. Resolve them first.'
  }
  return message
}
