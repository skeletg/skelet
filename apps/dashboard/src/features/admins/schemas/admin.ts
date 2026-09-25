import { z } from 'zod'
import type { SectionDiffEntry } from '@/shared/saving/saveRegistry'
import { added, fieldDiff, removed } from '@/shared/saving/diff'
import { email, newPassword } from '@/shared/schemas/fields'

// Admin dialog. Email and password are set once on create; an edit changes the profile, roles and
// active flag only. Dialogs keep no stored draft, so the password never reaches storage

export type ExistingAdmin = {
  id: string
  email: string
  firstName: string
  lastName: string
  isActive: boolean
  isSuperuser: boolean
  roles: { slug: string }[]
}

const shape = {
  firstName: z.string().trim().min(1, 'First name is required'),
  lastName: z.string().trim().min(1, 'Last name is required'),
  email: z.string(),
  password: z.string(),
  isActive: z.boolean(),
  roleSlugs: z.array(z.string()),
}

export const createAdminSchema = z.object({
  ...shape,
  email: email(),
  password: newPassword(),
})
export const editAdminSchema = z.object(shape)

export type AdminValues = z.infer<typeof editAdminSchema>

export function toAdminValues(admin?: ExistingAdmin): AdminValues {
  return {
    firstName: admin?.firstName ?? '',
    lastName: admin?.lastName ?? '',
    email: admin?.email ?? '',
    password: '',
    isActive: admin?.isActive ?? true,
    roleSlugs: admin?.roles.map((r) => r.slug) ?? [],
  }
}

// The admin's own page - the same profile and roles, never the email or password, which are the
// admin's own to change
export const adminDetailSchema = editAdminSchema.omit({ email: true, password: true })

export type AdminDetailValues = z.infer<typeof adminDetailSchema>

export const ADMIN_DETAIL_LABELS: Record<keyof AdminDetailValues, string> = {
  firstName: 'First name',
  lastName: 'Last name',
  isActive: 'Active',
  roleSlugs: 'Roles',
}

export function toAdminDetailValues(admin: ExistingAdmin): AdminDetailValues {
  return {
    firstName: admin.firstName,
    lastName: admin.lastName,
    isActive: admin.isActive,
    roleSlugs: admin.roles.map((r) => r.slug),
  }
}

// Roles only go when they changed - a superuser has none to edit, and the list is replaced whole
export function toAdminUpdateInput(v: AdminDetailValues, before: AdminDetailValues) {
  return {
    firstName: v.firstName.trim(),
    lastName: v.lastName.trim(),
    isActive: v.isActive,
    ...(sameRoles(v.roleSlugs, before.roleSlugs) ? {} : { roleSlugs: v.roleSlugs }),
  }
}

function sameRoles(a: string[], b: string[]) {
  return a.length === b.length && a.every((s) => b.includes(s))
}

export function adminDetailDiff(original: AdminDetailValues, draft: AdminDetailValues): SectionDiffEntry[] {
  return [
    ...fieldDiff(original, draft, [
      { label: ADMIN_DETAIL_LABELS.firstName, value: (v) => v.firstName },
      { label: ADMIN_DETAIL_LABELS.lastName, value: (v) => v.lastName },
      { label: ADMIN_DETAIL_LABELS.isActive, value: (v) => v.isActive },
    ]),
    ...[...draft.roleSlugs].sort().filter((s) => !original.roleSlugs.includes(s)).map((s) => added(`Role · ${s}`)),
    ...[...original.roleSlugs].sort().filter((s) => !draft.roleSlugs.includes(s)).map((s) => removed(`Role · ${s}`)),
  ]
}
