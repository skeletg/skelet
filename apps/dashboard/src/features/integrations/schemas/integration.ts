import { z } from 'zod'
import { isoToLocalInput, localInputToISO } from '@/shared/schemas/fields'
import type { SectionDiffEntry } from '@/shared/saving/saveRegistry'
import { added, fieldDiff, removed } from '@/shared/saving/diff'

// Integration (API token) dialog. The token itself is shown once, after create - nothing here
// holds it

export type ExistingIntegration = {
  id: string
  name: string
  description: string
  expiresAt?: string | null
  channelId?: string | null
  permissions: { slug: string }[]
}

export const integrationSchema = z
  .object({
    name: z.string().trim().min(1, 'Name is required'),
    description: z.string(),
    permissionSlugs: z.array(z.string()),
    // '' = a global token, else the one channel it's pinned to
    channelId: z.string(),
    hasExpiry: z.boolean(),
    // datetime-local text, "2026-05-01T12:30" - local time, converted to ISO on the way out
    expiresAt: z.string(),
  })
  .refine((v) => !v.hasExpiry || localInputToISO(v.expiresAt) !== null, {
    message: 'Pick when the token expires',
    path: ['expiresAt'],
  })

export type IntegrationValues = z.infer<typeof integrationSchema>

export function toIntegrationValues(integration?: ExistingIntegration): IntegrationValues {
  const expiresAt = isoToLocalInput(integration?.expiresAt)
  return {
    name: integration?.name ?? '',
    description: integration?.description ?? '',
    permissionSlugs: integration?.permissions.map((p) => p.slug) ?? [],
    channelId: integration?.channelId ?? '',
    hasExpiry: expiresAt !== '',
    expiresAt,
  }
}

function expiryIso(v: IntegrationValues): string | null {
  return v.hasExpiry ? localInputToISO(v.expiresAt) : null
}

export function toIntegrationCreateInput(v: IntegrationValues) {
  return {
    name: v.name.trim(),
    description: v.description,
    permissionSlugs: v.permissionSlugs,
    expiresAt: expiryIso(v),
    channelId: v.channelId || null,
  }
}

// An update clears what was switched off explicitly rather than leaving it out
export function toIntegrationUpdateInput(v: IntegrationValues, id: string) {
  return {
    id,
    name: v.name.trim(),
    description: v.description,
    permissionSlugs: v.permissionSlugs,
    ...(v.hasExpiry ? { expiresAt: expiryIso(v) } : { clearExpiresAt: true }),
    ...(v.channelId ? { channelId: v.channelId } : { clearChannel: true }),
  }
}

// The integration's own page - profile, expiry, permissions and the active switch. The channel pin is
// set on create and not edited here, so the update leaves it alone
export const integrationDetailSchema = z
  .object({
    isActive: z.boolean(),
    name: z.string().trim().min(1, 'Name is required'),
    description: z.string(),
    hasExpiry: z.boolean(),
    expiresAt: z.string(),
    permissionSlugs: z.array(z.string()),
  })
  .refine((v) => !v.hasExpiry || localInputToISO(v.expiresAt) !== null, {
    message: 'Pick when the token expires',
    path: ['expiresAt'],
  })

export type IntegrationDetailValues = z.infer<typeof integrationDetailSchema>

export const INTEGRATION_DETAIL_LABELS: Record<keyof IntegrationDetailValues, string> = {
  isActive: 'Active',
  name: 'Name',
  description: 'Description',
  hasExpiry: 'Expires',
  expiresAt: 'Expires at',
  permissionSlugs: 'Permissions',
}

export function toIntegrationDetailValues(integration: ExistingIntegration & { isActive: boolean }): IntegrationDetailValues {
  const v = toIntegrationValues(integration)
  return {
    isActive: integration.isActive,
    name: v.name,
    description: v.description,
    hasExpiry: v.hasExpiry,
    expiresAt: v.expiresAt,
    permissionSlugs: v.permissionSlugs,
  }
}

// The expiry as the datetime-local input edits it, to the minute - '' when there is none, so stored
// seconds never read as a change
const expiryText = (v: IntegrationDetailValues) => (v.hasExpiry ? v.expiresAt : '')

// Everything but the active flag, which has its own mutation
export function integrationProfileChanged(before: IntegrationDetailValues, after: IntegrationDetailValues): boolean {
  return (
    before.name !== after.name ||
    before.description !== after.description ||
    expiryText(before) !== expiryText(after) ||
    before.permissionSlugs.length !== after.permissionSlugs.length ||
    after.permissionSlugs.some((s) => !before.permissionSlugs.includes(s))
  )
}

export function toIntegrationDetailInput(v: IntegrationDetailValues, id: string) {
  return {
    id,
    name: v.name.trim(),
    description: v.description,
    permissionSlugs: v.permissionSlugs,
    ...(v.hasExpiry ? { expiresAt: localInputToISO(v.expiresAt) } : { clearExpiresAt: true }),
  }
}

export function integrationDetailDiff(original: IntegrationDetailValues, draft: IntegrationDetailValues): SectionDiffEntry[] {
  return [
    ...fieldDiff(original, draft, [
      { label: INTEGRATION_DETAIL_LABELS.isActive, value: (v) => v.isActive },
      { label: INTEGRATION_DETAIL_LABELS.name, value: (v) => v.name },
      { label: INTEGRATION_DETAIL_LABELS.description, value: (v) => v.description },
      { label: INTEGRATION_DETAIL_LABELS.expiresAt, value: expiryText },
    ]),
    ...[...draft.permissionSlugs].sort().filter((s) => !original.permissionSlugs.includes(s)).map((s) => added(`Permission · ${s}`)),
    ...[...original.permissionSlugs].sort().filter((s) => !draft.permissionSlugs.includes(s)).map((s) => removed(`Permission · ${s}`)),
  ]
}
