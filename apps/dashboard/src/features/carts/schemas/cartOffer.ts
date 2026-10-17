import { z } from 'zod'
import type { AdminUpdateCartOfferInput, CartOfferLock } from '@/graphql/schema.types'
import { addressIssues, addressSchema, toAddressInput, toAddressValues, ADDRESS_FIELDS, type AddressValues } from '@/shared/schemas/address'
import { changed, sameSet } from '@/shared/saving/diff'
import type { SectionDiffEntry } from '@/shared/saving/saveRegistry'
import type { CartOfferFieldsFragment } from '../Carts.gql'

// An offer cart's settings as the cart page stages them - its terms, what the recipient can't change,
// and the checkout they start from. All of it saves through one adminUpdateCartOffer call

export const OFFER_LOCKS: { value: CartOfferLock; label: string; hint: string; shipping?: boolean }[] = [
  { value: 'LINES', label: 'Items and quantities', hint: 'The recipient can’t add, remove or change items.' },
  { value: 'BILLING', label: 'Billing address', hint: 'Fill it in below - the recipient checks out with it as is.' },
  { value: 'SHIPPING', label: 'Shipping address', hint: 'Fill it in below.', shipping: true },
  { value: 'DELIVERY', label: 'Delivery', hint: 'Choose each delivery below.', shipping: true },
  { value: 'COUPONS', label: 'Coupons, gift cards and store credit', hint: 'Nothing can be taken off the offer price.' },
]

const deliverySchema = z
  .object({
    groupKey: z.string().describe('Key of the delivery group the choice is for'),
    optionId: z.string().describe('ID of the delivery option'),
    carrierSlug: z.string().describe('Carrier slug of the chosen method'),
    methodCode: z.string().describe('Code of the chosen method'),
    pointId: z.string().describe('ID of the pickup point, empty when the option takes none'),
    pointName: z.string().describe('Name of the pickup point, for display'),
    label: z.string().describe('How the choice reads in the console, such as "Courier · Standard"'),
  })
  .describe('The delivery an admin chose for one group of the offer')

export const cartOfferSchema = z
  .object({
    expiresAt: z.string().describe('When the offer stops working, as a datetime-local value (YYYY-MM-DDTHH:mm, local time)'),
    message: z.string().max(2000, 'At most 2000 characters').describe('A note to the recipient, shown in the email and on the offer - empty for none'),
    showInAccount: z.boolean().describe('Whether a customer offer is also listed in the customer’s account, not only reachable by the link'),
    locks: z.array(z.enum(['LINES', 'BILLING', 'SHIPPING', 'DELIVERY', 'COUPONS'])).describe('The parts of the cart the recipient may not change'),
    hasShipping: z.boolean().describe('Whether a shipping address is filled in'),
    shipping: addressSchema.describe('The shipping address - only used when hasShipping is true'),
    billingSameAsShipping: z.boolean().describe('Whether billing uses the shipping address'),
    hasBilling: z.boolean().describe('Whether a separate billing address is filled in'),
    billing: addressSchema.describe('The billing address - only used when hasBilling is true and billing is not the shipping address'),
    paymentMethods: z
      .array(z.string())
      .describe('Provider slugs of the payment methods the recipient may choose from - one fixes it, empty leaves every method the store offers'),
    deliveries: z.array(deliverySchema).describe('The delivery chosen per group - empty to leave it to the recipient'),
  })
  .describe('An offer cart’s terms and the checkout its recipient starts from')
  .superRefine((v, ctx) => {
    if (!v.expiresAt || Number.isNaN(new Date(v.expiresAt).getTime())) {
      ctx.addIssue({ code: 'custom', path: ['expiresAt'], message: 'Pick when the offer expires' })
    }
    if (v.hasShipping) addressIssues(v.shipping, ['shipping'], ctx)
    if (v.hasBilling && !(v.billingSameAsShipping && v.hasShipping)) addressIssues(v.billing, ['billing'], ctx)
    if (v.locks.includes('SHIPPING') && !v.hasShipping) {
      ctx.addIssue({ code: 'custom', path: ['locks'], message: 'Fill in the shipping address to lock it' })
    }
    if (v.locks.includes('BILLING') && !v.hasBilling && !(v.billingSameAsShipping && v.hasShipping)) {
      ctx.addIssue({ code: 'custom', path: ['locks'], message: 'Fill in the billing address to lock it' })
    }
    if (v.locks.includes('DELIVERY') && v.deliveries.length === 0) {
      ctx.addIssue({ code: 'custom', path: ['locks'], message: 'Choose the delivery to lock it' })
    }
  })

export type CartOfferValues = z.infer<typeof cartOfferSchema>
export type OfferDelivery = CartOfferValues['deliveries'][number]

// datetime-local wants local wall time without a zone
export function toLocalInput(iso: string): string {
  const d = new Date(iso)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}

export function toOfferValues(offer: CartOfferFieldsFragment): CartOfferValues {
  const co = offer.checkout
  return {
    expiresAt: toLocalInput(offer.expiresAt),
    message: offer.message,
    showInAccount: offer.showInAccount,
    locks: [...offer.locks],
    hasShipping: !!co.shippingAddress,
    shipping: toAddressValues(co.shippingAddress),
    billingSameAsShipping: co.billingSameAsShipping,
    hasBilling: !!co.billingAddress,
    billing: toAddressValues(co.billingAddress),
    paymentMethods: [...offer.paymentMethods],
    deliveries: co.deliveries.map((d) => ({
      groupKey: d.groupKey,
      optionId: d.optionId,
      carrierSlug: d.carrierSlug,
      methodCode: d.methodCode,
      pointId: d.pointId ?? '',
      pointName: d.pointName ?? '',
      label: '',
    })),
  }
}

// What the recipient has nothing to deliver for loses its shipping parts on save, so a cart that
// stopped shipping can't keep a locked address nobody sees
export function toOfferInput(v: CartOfferValues, needsShipping: boolean): AdminUpdateCartOfferInput {
  const ships = needsShipping && v.hasShipping
  const billingIsShipping = ships && v.billingSameAsShipping
  return {
    expiresAt: new Date(v.expiresAt).toISOString(),
    message: v.message.trim(),
    showInAccount: v.showInAccount,
    locks: needsShipping ? v.locks : v.locks.filter((l) => l !== 'SHIPPING' && l !== 'DELIVERY'),
    shippingAddress: ships ? toAddressInput(v.shipping) : null,
    billingSameAsShipping: billingIsShipping,
    billingAddress: !billingIsShipping && v.hasBilling ? toAddressInput(v.billing) : null,
    paymentMethods: v.paymentMethods,
    deliveries: needsShipping
      ? v.deliveries.map((d) => ({
          groupKey: d.groupKey,
          optionId: d.optionId,
          carrierSlug: d.carrierSlug,
          methodCode: d.methodCode,
          pointId: d.pointId || null,
        }))
      : [],
  }
}

const lockLabel = (l: string) => OFFER_LOCKS.find((x) => x.value === l)?.label ?? l

function addressLine(on: boolean, a: AddressValues): string {
  if (!on) return ''
  return [`${a.firstName} ${a.lastName}`.trim(), a.street1, `${a.postalCode} ${a.city}`.trim(), a.countryCode]
    .map((s) => s.trim())
    .filter(Boolean)
    .join(', ')
}

function addressDiffers(onA: boolean, a: AddressValues, onB: boolean, b: AddressValues): boolean {
  if (onA !== onB) return true
  return onA && ADDRESS_FIELDS.some(({ key }) => a[key].trim() !== b[key].trim())
}

function deliveryLine(ds: OfferDelivery[]): string {
  return ds
    .map((d) => [d.label || d.methodCode, d.pointName].filter(Boolean).join(' at '))
    .join('; ')
}

// One readable change per field the admin touched - an address is one entry, not ten
// paymentLabels names methods the way the picker does - a slug stands in until the channel's names load
export function offerDiff(before: CartOfferValues, after: CartOfferValues, paymentLabels: Record<string, string> = {}): SectionDiffEntry[] {
  const paymentLabel = (slug: string) => paymentLabels[slug] ?? slug
  const out: (SectionDiffEntry | null)[] = [
    changed('Expires', before.expiresAt.replace('T', ' '), after.expiresAt.replace('T', ' ')),
    changed('Message', before.message, after.message),
    changed('Show in the customer’s account', before.showInAccount, after.showInAccount),
  ]
  if (!sameSet(before.locks, after.locks)) {
    out.push({ kind: 'change', label: 'Locked', before: before.locks.map(lockLabel).join(', '), after: after.locks.map(lockLabel).join(', ') })
  }
  if (addressDiffers(before.hasShipping, before.shipping, after.hasShipping, after.shipping)) {
    out.push({ kind: 'change', label: 'Shipping address', before: addressLine(before.hasShipping, before.shipping), after: addressLine(after.hasShipping, after.shipping) })
  }
  out.push(changed('Billing same as shipping', before.billingSameAsShipping, after.billingSameAsShipping))
  if (addressDiffers(before.hasBilling, before.billing, after.hasBilling, after.billing)) {
    out.push({ kind: 'change', label: 'Billing address', before: addressLine(before.hasBilling, before.billing), after: addressLine(after.hasBilling, after.billing) })
  }
  if (!sameSet(before.paymentMethods, after.paymentMethods)) {
    const list = (m: string[]) => (m.length === 0 ? 'Any the store offers' : m.map((s) => paymentLabel(s)).join(', '))
    out.push({ kind: 'change', label: 'Payment methods', before: list(before.paymentMethods), after: list(after.paymentMethods) })
  }
  const key = (ds: OfferDelivery[]) => JSON.stringify(ds.map((d) => [d.groupKey, d.optionId, d.carrierSlug, d.methodCode, d.pointId]).sort())
  if (key(before.deliveries) !== key(after.deliveries)) {
    out.push({ kind: 'change', label: 'Delivery', before: deliveryLine(before.deliveries), after: deliveryLine(after.deliveries) })
  }
  return out.filter((e): e is SectionDiffEntry => e !== null && (e.kind !== 'change' || e.before !== e.after))
}
