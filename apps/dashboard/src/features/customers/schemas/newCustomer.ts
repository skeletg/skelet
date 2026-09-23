import { z } from 'zod'

// New-customer page. Accounts are scoped to one store, so the header must point at a channel; the
// password is only ever typed here and is kept out of the stored draft

export const newCustomerSchema = z.object({
  firstName: z.string().trim().min(1, 'First name is required'),
  lastName: z.string().trim().min(1, 'Last name is required'),
  email: z.email('Enter a valid email address'),
  password: z.string().min(8, 'Minimum 8 characters'),
  phone: z.string(),
  isActive: z.boolean(),
  isEmailVerified: z.boolean(),
  // '' = the store's default group
  groupSlug: z.string(),
})

export type NewCustomerValues = z.infer<typeof newCustomerSchema>

export const emptyNewCustomer: NewCustomerValues = {
  firstName: '',
  lastName: '',
  email: '',
  password: '',
  phone: '',
  isActive: true,
  isEmailVerified: false,
  groupSlug: '',
}

export function toNewCustomerInput(v: NewCustomerValues, channelId: string, defaultGroupSlug?: string) {
  return {
    firstName: v.firstName.trim(),
    lastName: v.lastName.trim(),
    email: v.email.trim(),
    password: v.password,
    phone: v.phone.trim() || null,
    isActive: v.isActive,
    isEmailVerified: v.isEmailVerified,
    customerGroupSlug: v.groupSlug || defaultGroupSlug || null,
    channelId,
  }
}
