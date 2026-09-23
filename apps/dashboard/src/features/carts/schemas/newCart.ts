import { z } from 'zod'

// New-cart dialog - a cart for a guest, an existing customer, or a customer created inline. What's
// required depends on the mode

export const newCartSchema = z
  .object({
    mode: z.enum(['existing', 'new', 'guest']),
    customer: z.object({ id: z.string(), label: z.string() }).nullable(),
    email: z.string(),
    firstName: z.string(),
    lastName: z.string(),
  })
  .superRefine((v, ctx) => {
    if (v.mode === 'existing' && !v.customer) {
      ctx.addIssue({ code: 'custom', path: ['customer'], message: 'Pick a customer' })
    }
    if (v.mode !== 'new') return
    if (!z.email().safeParse(v.email.trim()).success) {
      ctx.addIssue({ code: 'custom', path: ['email'], message: 'Enter a valid email address' })
    }
    if (!v.firstName.trim()) ctx.addIssue({ code: 'custom', path: ['firstName'], message: 'First name is required' })
    if (!v.lastName.trim()) ctx.addIssue({ code: 'custom', path: ['lastName'], message: 'Last name is required' })
  })

export type NewCartValues = z.infer<typeof newCartSchema>

export const emptyNewCart: NewCartValues = { mode: 'existing', customer: null, email: '', firstName: '', lastName: '' }

// A strong throwaway password for an admin-created customer - they set their own via password reset
export function randomPassword(): string {
  const bytes = new Uint8Array(18)
  crypto.getRandomValues(bytes)
  return Array.from(bytes, (b) => b.toString(36).padStart(2, '0')).join('') + 'Aa1!'
}
