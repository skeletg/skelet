import { z } from 'zod'
import { addressIssues, addressSchema, toAddressInput, toAddressValues } from '@/shared/schemas/address'

// Manual (draft) order - a customer or a guest email, simple-product lines, addresses and an optional
// flat discount. Prices shown here are hints: the server re-derives them from the catalog

const lineSchema = z.object({
  sku: z.string(),
  name: z.string(),
  unitPrice: z.number(),
  currency: z.string(),
  quantity: z.number().positive('Above 0'),
})

export type DraftLine = z.infer<typeof lineSchema>
export type DraftCustomer = { id: string; email: string; label: string }

export const draftOrderSchema = z
  .object({
    customer: z.object({ id: z.string(), email: z.string(), label: z.string() }).nullable(),
    email: z.email('Enter the order email'),
    lines: z.array(lineSchema).min(1, 'Add at least one product'),
    billing: addressSchema,
    shipping: addressSchema,
    shipSame: z.boolean(),
    // Whole minor units (cents), clamped to the subtotal server-side
    discount: z.string().refine((v) => v.trim() === '' || /^\d+$/.test(v.trim()), 'Whole cents, 0 or more'),
    note: z.string(),
  })
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
  return Number.parseInt(v.discount.trim() || '0', 10) || 0
}

export function toDraftOrderInput(v: DraftOrderValues, channelId: string, sourceCartId: string | null) {
  const cents = discountCents(v)
  return {
    channelId,
    customerId: v.customer?.id ?? null,
    email: v.email.trim(),
    billingAddress: toAddressInput(v.billing),
    shippingAddress: toAddressInput(v.shipSame ? v.billing : v.shipping),
    lines: v.lines.map((l) => ({ productSku: l.sku, quantity: l.quantity })),
    manualDiscountCents: cents > 0 ? cents : null,
    customerNote: v.note.trim() || null,
    sourceCartId,
  }
}
