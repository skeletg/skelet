import { z } from 'zod'
import { numberText } from '@/shared/schemas/fields'
import type { WebhookFailurePolicy, WebhookKind } from '@/graphql/schema.types'
import type { SectionDiffEntry } from '@/shared/saving/saveRegistry'
import { fieldDiff } from '@/shared/saving/diff'
import type { UpdateWebhookMutationVariables } from '../Webhooks.gql'
import { headerRowsToObject, isWildcardEntry, objectToHeaderRows } from '../deliverySettings.utils'

// Webhook subscription - the create page and the edit dialog share this. Kind and channel scope are
// fixed at creation; an update sends only what moved

export type ExistingWebhook = {
  id: string
  name: string
  targetUrl: string
  eventTypes: string[]
  kind: WebhookKind
  channel?: { id: string; slug: string; name: string } | null
  failurePolicy?: WebhookFailurePolicy | null
  timeoutMs?: number | null
  customHeaders?: unknown
}

export const webhookSchema = z.object({
  name: z.string().trim().min(1, 'Name is required'),
  targetUrl: z.url('Enter a full URL, starting with https://'),
  kind: z.enum(['ASYNC', 'SYNC']),
  eventTypes: z.array(z.string()).min(1, 'Select at least one event type'),
  // '' = every store
  channelId: z.string(),
  failurePolicy: z.enum(['FAIL_OPEN', 'FAIL_CLOSED']),
  timeoutMs: numberText({ optional: true, int: true, min: 0, max: 30000 }),
  // Extra request headers - often auth tokens, so they're kept out of stored drafts
  headers: z.array(z.object({ key: z.string(), value: z.string() })),
})

export type WebhookValues = z.infer<typeof webhookSchema>

export const WEBHOOK_LABELS: Record<keyof WebhookValues, string> = {
  name: 'Name',
  targetUrl: 'Target URL',
  kind: 'Kind',
  eventTypes: 'Event types',
  channelId: 'Channel scope',
  failurePolicy: 'On failure',
  timeoutMs: 'Timeout',
  headers: 'Custom headers',
}

export function toWebhookValues(w?: ExistingWebhook): WebhookValues {
  return {
    name: w?.name ?? '',
    targetUrl: w?.targetUrl ?? '',
    // Async is the safer default - it can't block the operation it reports on
    kind: w?.kind ?? 'ASYNC',
    eventTypes: w?.eventTypes ?? [],
    channelId: w?.channel?.id ?? '',
    failurePolicy: w?.failurePolicy ?? 'FAIL_CLOSED',
    timeoutMs: w?.timeoutMs ? String(w.timeoutMs) : '',
    headers: objectToHeaderRows(w?.customHeaders),
  }
}

// Switching kind drops the events that belong to the other one - wildcards are kind-agnostic patterns
// and survive
export function keepEventsOfKind(selected: string[], allowed: ReadonlySet<string>): string[] {
  return selected.filter((t) => isWildcardEntry(t) || allowed.has(t))
}

export function toWebhookCreateInput(v: WebhookValues) {
  return {
    name: v.name.trim(),
    targetUrl: v.targetUrl.trim(),
    kind: v.kind,
    eventTypes: v.eventTypes,
    channelId: v.channelId || undefined,
    failurePolicy: v.failurePolicy,
    timeoutMs: v.timeoutMs.trim() === '' ? undefined : Number(v.timeoutMs),
    customHeaders: headerRowsToObject(v.headers),
  }
}

// Only what moved - null when nothing did. Channel reassignment isn't supported by the API (a
// webhook's channel is fixed at creation), so it's never sent
export function toWebhookUpdateInput(v: WebhookValues, w: ExistingWebhook): UpdateWebhookMutationVariables['input'] | null {
  const input: UpdateWebhookMutationVariables['input'] = { id: w.id }
  if (v.name.trim() !== w.name) input.name = v.name.trim()
  if (v.targetUrl.trim() !== w.targetUrl) input.targetUrl = v.targetUrl.trim()
  if (JSON.stringify(v.eventTypes) !== JSON.stringify(w.eventTypes)) input.eventTypes = v.eventTypes
  if (v.failurePolicy !== (w.failurePolicy ?? 'FAIL_CLOSED')) input.failurePolicy = v.failurePolicy
  const timeout = v.timeoutMs.trim() === '' ? 0 : Number(v.timeoutMs)
  if (timeout !== (w.timeoutMs ?? 0)) input.timeoutMs = timeout
  const next = headerRowsToObject(v.headers) ?? {}
  const prev = headerRowsToObject(objectToHeaderRows(w.customHeaders)) ?? {}
  if (JSON.stringify(next) !== JSON.stringify(prev)) input.customHeaders = next // {} clears server-side
  return Object.keys(input).length > 1 ? input : null
}

// For the create page's restored-draft banner - the fields as a reviewer would name them
export function webhookDiff(a: WebhookValues, b: WebhookValues): SectionDiffEntry[] {
  return fieldDiff(a, b, [
    { label: WEBHOOK_LABELS.name, value: (o) => o.name },
    { label: WEBHOOK_LABELS.targetUrl, value: (o) => o.targetUrl },
    { label: WEBHOOK_LABELS.eventTypes, value: (o) => o.eventTypes },
    { label: WEBHOOK_LABELS.failurePolicy, value: (o) => o.failurePolicy },
    { label: WEBHOOK_LABELS.timeoutMs, value: (o) => o.timeoutMs },
  ])
}
