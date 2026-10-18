import { z } from 'zod'
import { email, numberText, parseNumberText } from '@/shared/schemas/fields'
import { addressIssues, addressSchema, toAddressInput, toAddressValues } from '@/shared/schemas/address'

// Manual (draft) order - a customer or a guest email, simple-product lines, addresses and an optional
// flat discount. Prices shown here are hints: the server re-derives them from the catalog

const lineSchema = z
  .object({
    sku: z.string().describe('SKU of the product - one line per SKU'),
    name: z.string().describe('Product name, for display'),
    unitPrice: z.number().describe('Unit price in minor units (cents), for display - the server re-derives it'),
    currency: z.string().describe('ISO 4217 code of unitPrice, such as USD'),
    quantity: z.number().positive('Above 0').describe('Units ordered, above 0'),
  })
  .describe('One product line')

export type DraftLine = z.infer<typeof lineSchema>
export type DraftCustomer = { id: string; email: string; label: string }

export const draftOrderSchema = z
  .object({
    customer: z
      .object({
        id: z.string().describe('ID of the customer'),
        email: z.string().describe("Customer's email"),
        label: z.string().describe('Name or email the customer is shown by'),
      })
      .nullable()
      .describe('The customer placing the order, or null for a guest order'),
    email: email('Enter the order email').describe('Email the order confirmation goes to'),
    lines: z.array(lineSchema).min(1, 'Add at least one product').describe('Product lines, at least one'),
    billing: addressSchema.describe('Billing address - names may be left empty'),
    shipping: addressSchema.describe('Shipping address - ignored when shipSame is on, names may be left empty'),
    shipSame: z.boolean().describe('Ship to the billing address'),
    discount: numberText({ optional: true, int: true, min: 0 }, 'Whole cents, 0 or more').describe(
      'Flat discount in whole minor units (cents) written as text, capped at the subtotal - empty string for none',
    ),
    note: z.string().describe('Note on the order - empty string for none'),
  })
  .describe('A manual (draft) order placed by an admin')
  .superRefine((v, ctx) => {
    // A draft needs where it goes - the names can follow, as they always could here
    addressIssues(v.billing, ['billing'], ctx, { requireNames: false })
    if (!v.shipSame) addressIssues(v.shipping, ['shipping'], ctx, { requireNames: false })
  })

export type DraftOrderValues = z.infer<typeof draftOrderSchema>

export function toDraftOrderValues(initial: { customer: DraftCustomer | null; email: string; lines: DraftLine[] }): DraftOrderValues {
  return {
    customer: initial.customer,
    email: initial.email,
    lines: initial.lines,
    billing: toAddressValues(),
    shipping: toAddressValues(),
    shipSame: true,
    discount: '',
    note: '',
  }
}

// Adding a product already on the order bumps its quantity instead of adding a second line
export function addDraftLine(lines: DraftLine[], next: Omit<DraftLine, 'quantity'>): DraftLine[] {
  return lines.some((l) => l.sku === next.sku)
    ? lines.map((l) => (l.sku === next.sku ? { ...l, quantity: l.quantity + 1 } : l))
    : [...lines, { ...next, quantity: 1 }]
}

export function discountCents(v: DraftOrderValues): number {
  return parseNumberText(v.discount) ?? 0
}

export function toDraftOrderInput(v: DraftOrderValues, channelId: string, sourceCartId: string | null) {
  const cents = discountCents(v)
  return {
    channelId,
    customerId: v.customer?.id ?? null,
    email: v.email.trim(),
    billingAddress: toAddressInput(v.billing),
    shippingAddress: toAddressInput(v.shipSame ? v.billing : v.shipping),
    lines: v.lines.map((l) => ({ sku: l.sku, quantity: l.quantity })),
    manualDiscountCents: cents > 0 ? cents : null,
    customerNote: v.note.trim() || null,
    sourceCartId,
  }
}
