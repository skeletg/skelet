import { z } from 'zod'

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
  email: z.email('Enter a valid email address'),
  password: z.string().min(8, 'Minimum 8 characters'),
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
