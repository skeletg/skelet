import { z } from 'zod'
import { numberText } from '@/shared/schemas/fields'
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

export const remoteProviderSchema = z.object({
  kind: z.string(),
  slug: z.string().trim().min(1, 'Slug is required'),
  name: z.string().trim().min(1, 'Name is required'),
  targetUrl: z.url('Enter a full URL, starting with https://'),
  channelId: z.string(),
  config: z.string().refine((v) => parseJson(v).ok, 'Config must be valid JSON'),
  timeoutMs: boundedInt(30000),
  cacheTtlS: boundedInt(3600),
  failurePolicy: z.enum(['FAIL_OPEN', 'FAIL_CLOSED']),
})

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
