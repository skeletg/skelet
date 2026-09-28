import { z } from 'zod'
import { email, newPassword } from '@/shared/schemas/fields'

// New-customer page. Accounts are scoped to one store, so the header must point at a channel; the
// password is only ever typed here and is kept out of the stored draft

export const newCustomerSchema = z
  .object({
    firstName: z.string().trim().min(1, 'First name is required').describe("Customer's first name"),
    lastName: z.string().trim().min(1, 'Last name is required').describe("Customer's last name"),
    email: email().describe("Customer's sign-in email address"),
    password: newPassword(),
    phone: z.string().describe('Phone number - empty string for none'),
    isActive: z.boolean().describe('Whether the customer can sign in'),
    isEmailVerified: z.boolean().describe('Mark the email address as already verified'),
    groupSlug: z.string().describe("Slug of the customer group to join - empty string for the store's default group"),
  })
  .describe('A new customer account in the current store')

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
