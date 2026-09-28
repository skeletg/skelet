import { z } from 'zod'
import { absoluteUrl } from '@/shared/schemas/fields'
import type { EventDestinationInput } from '@/graphql/schema.types'

// Analytics destination dialog. The signing secret is write-only: never shown again, so an edit
// leaves it blank to keep the stored one and only a new webhook destination has to supply it

export type ExistingDestination = {
  id: string
  name: string
  kind: string
  targetUrl: string
  eventSources: string[]
  eventNames: string[]
  isActive: boolean
}

// The analytics spine's source column - filters match against these
export const EVENT_SOURCES = ['campaign', 'flow', 'transactional', 'web', 'system'] as const

export function destinationSchema(opts: { isEdit: boolean }) {
  return z
    .object({
      name: z.string().trim().min(1, 'Name is required').describe('Display name of the destination'),
      kind: z.string().describe('Where events are sent: webhook, ga4_mp (Google Analytics 4) or meta_capi (Meta Conversions API)'),
      targetUrl: absoluteUrl().describe('Absolute URL events are delivered to'),
      secret: z
        .string()
        .describe(
          opts.isEdit
            ? 'New HMAC signing secret for webhook deliveries - write-only, empty string keeps the stored one'
            : 'HMAC signing secret for webhook deliveries, write-only - required for a webhook destination',
        ),
      eventSources: z
        .array(z.string())
        .describe(`Event sources to forward, from ${EVENT_SOURCES.join(', ')} - empty for every source`),
      eventNames: z.string().describe('Comma-separated event names to forward - empty string for every event'),
      isActive: z.boolean().describe('Whether events are being delivered'),
    })
    .describe('An analytics destination that analytics events are forwarded to')
    .refine((v) => opts.isEdit || v.kind !== 'webhook' || v.secret.trim() !== '', {
      message: 'A new webhook destination needs a signing secret',
      path: ['secret'],
    })
}

export type DestinationValues = z.infer<ReturnType<typeof destinationSchema>>

export function toDestinationValues(d?: ExistingDestination | null): DestinationValues {
  return {
    name: d?.name ?? '',
    kind: d?.kind ?? 'webhook',
    targetUrl: d?.targetUrl ?? '',
    secret: '',
    eventSources: d?.eventSources ?? [],
    eventNames: d?.eventNames.join(', ') ?? '',
    isActive: d?.isActive ?? true,
  }
}

export function toDestinationInput(v: DestinationValues, channelId: string): EventDestinationInput {
  return {
    channelId,
    name: v.name.trim(),
    kind: v.kind,
    targetUrl: v.targetUrl.trim(),
    // Blank keeps the stored secret on edit
    secret: v.secret.trim() === '' ? undefined : v.secret.trim(),
    eventSources: v.eventSources,
    eventNames: v.eventNames
      .split(',')
      .map((n) => n.trim())
      .filter(Boolean),
    isActive: v.isActive,
  }
}
