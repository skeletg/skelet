import { z } from 'zod'
import type { SectionDiffEntry } from '@/shared/saving/saveRegistry'
import { added, fieldDiff, removed } from '@/shared/saving/diff'
import type { RoleQuery } from '../Roles.gql'
import type { Scope } from '../useRoleDraft'

// Admin role - THE field list (docs/conventions.md, "Record forms"). Identity plus the grant list; the
// slug is set on create only, since changing it would orphan every audit entry that names the role

const scopeSchema = z
  .object({
    permissionId: z.string().describe('ID of the granted permission'),
    permissionSlug: z.string().describe('Slug of the granted permission, such as products.update'),
    channelId: z.string().nullable().describe('The channel the grant is limited to, or null for every channel'),
    locale: z.string().nullable().describe('The locale the grant is limited to, or null for every locale'),
  })
  .describe('One permission grant, optionally narrowed to a channel and a locale')

export const roleSchema = z
  .object({
    slug: z.string().trim().min(1, 'Slug is required').describe('Stable identifier for the role - set on create, never changed'),
    name: z.string().trim().min(1, 'Name is required').describe('Display name of the role'),
    description: z.string().describe('What the role is for - empty string for none'),
    scopes: z
      .array(scopeSchema)
      .min(1, 'A role with zero grants does nothing - grant at least one permission')
      .describe('The permissions the role grants'),
  })
  .describe('An admin role: a named set of permission grants')

export type RoleValues = z.infer<typeof roleSchema>

export const ROLE_LABELS: Record<keyof RoleValues, string> = {
  slug: 'Slug',
  name: 'Name',
  description: 'Description',
  scopes: 'Grants',
}

type ExistingRole = NonNullable<RoleQuery['role']>

export function toRoleValues(role?: ExistingRole): RoleValues {
  return {
    slug: role?.slug ?? '',
    name: role?.name ?? '',
    description: role?.description ?? '',
    scopes: (role?.scopes ?? []).map((s) => ({
      permissionId: s.permission.id,
      permissionSlug: s.permission.slug,
      channelId: s.channelId ?? null,
      locale: s.locale ?? null,
    })),
  }
}

function toScopesInput(scopes: Scope[]) {
  return scopes.map((s) => ({ permissionSlug: s.permissionSlug, channelId: s.channelId, locale: s.locale }))
}

export function toCreateRoleInput(v: RoleValues) {
  return { slug: v.slug.trim(), name: v.name.trim(), description: v.description.trim() || null, scopes: toScopesInput(v.scopes) }
}

export function toUpdateRoleInput(v: RoleValues) {
  return { name: v.name.trim(), description: v.description.trim() || null, scopes: toScopesInput(v.scopes) }
}

const grantKey = (s: Scope) => `${s.permissionSlug}|${s.channelId ?? ''}|${s.locale ?? ''}`

// One entry per grant added or removed - a grant is its (permission, channel, locale) tuple, so moving
// one to another channel reads as the old one removed and the new one added
export function roleDiff(channelName: (id: string) => string) {
  const describe = (s: Scope) =>
    [s.permissionSlug, s.channelId ? channelName(s.channelId) : 'any channel', s.locale ?? 'any locale'].join(' · ')
  return (original: RoleValues, draft: RoleValues): SectionDiffEntry[] => {
    const before = new Set(original.scopes.map(grantKey))
    const after = new Set(draft.scopes.map(grantKey))
    return [
      ...fieldDiff(original, draft, [
        { label: ROLE_LABELS.name, value: (v) => v.name },
        { label: ROLE_LABELS.description, value: (v) => v.description },
      ]),
      ...original.scopes.filter((s) => !after.has(grantKey(s))).map((s) => removed('Grant', describe(s))),
      ...draft.scopes.filter((s) => !before.has(grantKey(s))).map((s) => added('Grant', describe(s))),
    ]
  }
}
