import { z } from 'zod'
import { EMAIL_MESSAGE, isEmail } from '@/shared/schemas/fields'

// New-cart dialog - a cart for a guest, an existing customer, or a customer created inline. What's
// required depends on the mode

export const newCartSchema = z
  .object({
    mode: z
      .enum(['existing', 'new', 'guest'])
      .describe('Who the cart is for: an existing customer, a customer created along with it, or a guest'),
    customer: z
      .object({
        id: z.string().describe('ID of the customer'),
        label: z.string().describe('Name or email the customer is shown by'),
      })
      .nullable()
      .describe('The customer the cart belongs to - required in existing mode, null otherwise'),
    email: z.string().describe('Email of the customer to create - required in new mode'),
    firstName: z.string().describe('First name of the customer to create - required in new mode'),
    lastName: z.string().describe('Last name of the customer to create - required in new mode'),
    offer: z
      .boolean()
      .describe('Build it as an offer - a separate cart sent to the recipient as a link, never their own cart'),
    guestEmail: z.string().describe('Who a guest offer is sent to - required for an offer in guest mode'),
  })
  .describe('A new cart, started by an admin on behalf of a shopper')
  .superRefine((v, ctx) => {
    if (v.mode === 'existing' && !v.customer) {
      ctx.addIssue({ code: 'custom', path: ['customer'], message: 'Pick a customer' })
    }
    if (v.offer && v.mode === 'guest' && !isEmail(v.guestEmail)) {
      ctx.addIssue({ code: 'custom', path: ['guestEmail'], message: EMAIL_MESSAGE })
    }
    if (v.mode !== 'new') return
    if (!isEmail(v.email)) {
      ctx.addIssue({ code: 'custom', path: ['email'], message: EMAIL_MESSAGE })
    }
    if (!v.firstName.trim()) ctx.addIssue({ code: 'custom', path: ['firstName'], message: 'First name is required' })
    if (!v.lastName.trim()) ctx.addIssue({ code: 'custom', path: ['lastName'], message: 'Last name is required' })
  })

export type NewCartValues = z.infer<typeof newCartSchema>

export const emptyNewCart: NewCartValues = {
  mode: 'existing',
  customer: null,
  email: '',
  firstName: '',
  lastName: '',
  offer: false,
  guestEmail: '',
}

// A strong throwaway password for an admin-created customer - they set their own via password reset
export function randomPassword(): string {
  const bytes = new Uint8Array(18)
  crypto.getRandomValues(bytes)
  return Array.from(bytes, (b) => b.toString(36).padStart(2, '0')).join('') + 'Aa1!'
}
