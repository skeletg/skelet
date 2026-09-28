import { z } from 'zod'
import { addressChanged, addressIssues, addressSchema, toAddressInput, type AddressValues } from '@/shared/schemas/address'

// Edit-addresses dialog on a placed, pre-fulfillment order. Only the side that changed is validated
// and sent: a shipping change makes the backend recompute tax and re-quote shipping, a billing-only
// edit leaves the money alone

export type OrderAddressValues = { shipping: AddressValues; billing: AddressValues }

export function orderAddressSchema(stored: OrderAddressValues) {
  return z
    .object({
      shipping: addressSchema.describe('Shipping address - a change recomputes tax and re-quotes shipping'),
      billing: addressSchema.describe('Billing address'),
    })
    .describe("A placed order's shipping and billing addresses, before fulfilment")
    .superRefine((v, ctx) => {
      if (addressChanged(v.shipping, stored.shipping)) addressIssues(v.shipping, ['shipping'], ctx)
      if (addressChanged(v.billing, stored.billing)) addressIssues(v.billing, ['billing'], ctx)
    })
}

export function toOrderAddressInput(v: OrderAddressValues, stored: OrderAddressValues, orderId: string) {
  const shippingChanged = addressChanged(v.shipping, stored.shipping)
  const billingChanged = addressChanged(v.billing, stored.billing)
  return {
    shippingChanged,
    billingChanged,
    input: {
      orderId,
      shippingAddress: shippingChanged ? toAddressInput(v.shipping) : null,
      billingAddress: billingChanged ? toAddressInput(v.billing) : null,
    },
  }
}
