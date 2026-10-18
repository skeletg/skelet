import { z } from 'zod'
import { absoluteUrl } from '@/shared/schemas/fields'
import type { EventDestinationInput } from '@/graphql/schema.types'

// Analytics destination dialog. The secret is write-only: never shown again, so an edit leaves it blank to keep
// the stored one and only a new destination has to supply it - the webhook's signing key, GA4's API secret or Meta's
// access token, which the forwarder adds to the URL at send time so the URL itself never holds a credential

export type ExistingDestination = {
  id: string
  name: string
  kind: string
  targetUrl: string
  eventSources: string[]
  eventNames: string[]
  isActive: boolean
}

const SECRET_FIELDS = {
  webhook: { label: 'Signing secret', placeholder: 'HMAC key' },
  ga4_mp: { label: 'API secret', placeholder: 'Measurement Protocol API secret' },
  meta_capi: { label: 'Access token', placeholder: 'Conversions API access token' },
}

export function secretField(kind: string) {
  return SECRET_FIELDS[kind as keyof typeof SECRET_FIELDS] ?? SECRET_FIELDS.webhook
}

const CREDENTIAL_PARAMS = ['api_secret', 'access_token']

function credentialInUrl(raw: string): string | undefined {
  try {
    const params = new URL(raw).searchParams
    return CREDENTIAL_PARAMS.find((p) => params.has(p))
  } catch {
    return undefined
  }
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
            ? 'New write-only secret (webhook HMAC key, GA4 api_secret or Meta access token) - empty string keeps the stored one'
            : 'Write-only secret, required: the webhook HMAC key, the GA4 api_secret or the Meta access token',
        ),
      eventSources: z
        .array(z.string())
        .describe(`Event sources to forward, from ${EVENT_SOURCES.join(', ')} - empty for every source`),
      eventNames: z.string().describe('Comma-separated event names to forward - empty string for every event'),
      isActive: z.boolean().describe('Whether events are being delivered'),
    })
    .describe('An analytics destination that analytics events are forwarded to')
    .superRefine((v, ctx) => {
      if (!opts.isEdit && v.secret.trim() === '') {
        ctx.addIssue({ code: 'custom', path: ['secret'], message: `A new destination needs its ${secretField(v.kind).label.toLowerCase()}` })
      }
      const param = credentialInUrl(v.targetUrl)
      if (param) {
        ctx.addIssue({ code: 'custom', path: ['targetUrl'], message: `Put the ${param} in the ${secretField(v.kind).label.toLowerCase()} field, not the URL` })
      }
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
