import { z } from 'zod'
import { absoluteUrl, numberText } from '@/shared/schemas/fields'
import type { RemoteProviderFailurePolicy, RemoteProviderKind } from '@/graphql/schema.types'
import type { UpdateRemoteProviderMutationVariables } from '../RemoteProviders.gql'

// Remote provider dialog - an HTTP endpoint implementing a pluggable role. Kind, slug and channel
// scope are fixed at creation; an update sends only the fields that moved

export type ExistingProvider = {
  id: string
  kind: RemoteProviderKind
  slug: string
  name: string
  targetUrl: string
  config?: unknown
  timeoutMs: number
  cacheTtlS: number
  failurePolicy: RemoteProviderFailurePolicy
  channel?: { id: string; slug: string; name: string } | null
}

function parseJson(text: string): { ok: true; value: unknown } | { ok: false } {
  if (!text.trim()) return { ok: true, value: undefined }
  try {
    return { ok: true, value: JSON.parse(text) }
  } catch {
    return { ok: false }
  }
}

// Blank means "use the default" - otherwise a whole number within the service's bounds
const boundedInt = (max: number) => numberText({ optional: true, int: true, min: 0, max })

export const remoteProviderSchema = z
  .object({
    kind: z
      .string()
      .describe(
        'The role the endpoint implements: SHIPPING_CARRIER, TAX_PROVIDER, ADDRESS_VALIDATOR, PROMOTION_CONDITION, PROMOTION_ACTION, FRAUD_RISK, CURRENCY_RATE or PRICING - set on create, never changed',
      ),
    slug: z
      .string()
      .trim()
      .min(1, 'Slug is required')
      .describe('Stable identifier consumers such as promotion rules reference - set on create, never changed'),
    name: z.string().trim().min(1, 'Name is required').describe('Display name of the provider'),
    targetUrl: absoluteUrl().describe('Absolute URL of the endpoint core calls'),
    channelId: z.string().describe('ID of the one channel the provider serves, set on create - empty string for every channel'),
    config: z
      .string()
      .refine((v) => parseJson(v).ok, 'Config must be valid JSON')
      .describe('JSON text forwarded verbatim with every call, such as API keys or options - empty string for none'),
    timeoutMs: boundedInt(30000).describe(
      'Per-call HTTP timeout in milliseconds, up to 30000, written as text - empty string or 0 uses the 5 second default',
    ),
    cacheTtlS: boundedInt(3600).describe(
      'Seconds to cache a successful response per request payload, up to 3600, written as text - empty string or 0 disables caching',
    ),
    failurePolicy: z
      .enum(['FAIL_OPEN', 'FAIL_CLOSED'])
      .describe('When a call fails: FAIL_OPEN falls back to core behaviour, FAIL_CLOSED aborts the operation'),
  })
  .describe('A remote provider: an HTTP endpoint that implements a pluggable role, such as shipping rates or tax')

export type RemoteProviderValues = z.infer<typeof remoteProviderSchema>

export function toRemoteProviderValues(p?: ExistingProvider): RemoteProviderValues {
  return {
    kind: p?.kind ?? 'SHIPPING_CARRIER',
    slug: p?.slug ?? '',
    name: p?.name ?? '',
    targetUrl: p?.targetUrl ?? '',
    // '' = every store
    channelId: p?.channel?.id ?? '',
    config: p?.config ? JSON.stringify(p.config, null, 2) : '',
    timeoutMs: p?.timeoutMs ? String(p.timeoutMs) : '',
    cacheTtlS: p?.cacheTtlS ? String(p.cacheTtlS) : '',
    failurePolicy: p?.failurePolicy ?? 'FAIL_OPEN',
  }
}

const config = (v: RemoteProviderValues) => {
  const parsed = parseJson(v.config)
  return parsed.ok ? parsed.value : undefined
}

export function toRemoteProviderCreateInput(v: RemoteProviderValues) {
  return {
    kind: v.kind as RemoteProviderKind,
    slug: v.slug.trim(),
    name: v.name.trim(),
    targetUrl: v.targetUrl.trim(),
    config: config(v),
    timeoutMs: v.timeoutMs.trim() === '' ? undefined : Number(v.timeoutMs),
    cacheTtlS: v.cacheTtlS.trim() === '' ? undefined : Number(v.cacheTtlS),
    failurePolicy: v.failurePolicy,
    channelId: v.channelId || undefined,
  }
}

// Only what moved, so an untouched field can't overwrite a concurrent change - null when nothing did
export function toRemoteProviderUpdateInput(
  v: RemoteProviderValues,
  existing: ExistingProvider,
): UpdateRemoteProviderMutationVariables['input'] | null {
  const input: UpdateRemoteProviderMutationVariables['input'] = { id: existing.id }
  if (v.name.trim() !== existing.name) input.name = v.name.trim()
  if (v.targetUrl.trim() !== existing.targetUrl) input.targetUrl = v.targetUrl.trim()
  const next = config(v)
  if ((next === undefined ? '' : JSON.stringify(next)) !== (existing.config ? JSON.stringify(existing.config) : '')) {
    input.config = next ?? {}
  }
  const timeout = v.timeoutMs.trim() === '' ? 0 : Number(v.timeoutMs)
  if (timeout !== existing.timeoutMs) input.timeoutMs = timeout
  const ttl = v.cacheTtlS.trim() === '' ? 0 : Number(v.cacheTtlS)
  if (ttl !== existing.cacheTtlS) input.cacheTtlS = ttl
  if (v.failurePolicy !== existing.failurePolicy) input.failurePolicy = v.failurePolicy
  return Object.keys(input).length > 1 ? input : null
}
