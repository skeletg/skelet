import { z } from 'zod'

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
  .refine((v) => !v.hasExpiry || (v.expiresAt !== '' && !Number.isNaN(new Date(v.expiresAt).getTime())), {
    message: 'Pick when the token expires',
    path: ['expiresAt'],
  })

export type IntegrationValues = z.infer<typeof integrationSchema>

// The datetime-local input speaks local "YYYY-MM-DDTHH:mm", the API speaks ISO
function toLocalInput(iso: string | null | undefined): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}

export function toIntegrationValues(integration?: ExistingIntegration): IntegrationValues {
  const expiresAt = toLocalInput(integration?.expiresAt)
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
  return v.hasExpiry && v.expiresAt ? new Date(v.expiresAt).toISOString() : null
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
