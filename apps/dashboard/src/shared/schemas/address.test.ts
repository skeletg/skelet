import { describe, expect, it } from 'vitest'
import { z } from 'zod'
import { addressIssues, addressSchema, toAddressValues } from './address'

const schema = addressSchema.superRefine((a, ctx) => addressIssues(a, [], ctx))

const berlin = toAddressValues({
  firstName: 'Ana',
  lastName: 'Ilić',
  street1: 'Unter den Linden 1',
  city: 'Berlin',
  postalCode: '10117',
  countryCode: 'DE',
})

const issuesOf = (v: z.input<typeof schema>) => schema.safeParse(v).error?.issues.map((i) => `${i.path.join('.')}: ${i.message}`)

describe('addressIssues phone', () => {
  it("reads the phone in the address's own country", () => {
    expect(schema.safeParse({ ...berlin, phone: '01512 3456789' }).success).toBe(true)
    expect(issuesOf({ ...berlin, countryCode: 'US', phone: '01512 3456789' })).toEqual([
      'phone: Not a valid phone number for US - or start it with + and the country code',
    ])
  })

  it('takes an international number on any address, and no phone at all', () => {
    expect(schema.safeParse({ ...berlin, phone: '+381 64 123 4567' }).success).toBe(true)
    expect(schema.safeParse(berlin).success).toBe(true)
  })

  it('files the issue under where the address sits in the form', () => {
    const nested = z
      .object({ shipping: addressSchema })
      .superRefine((v, ctx) => addressIssues(v.shipping, ['shipping'], ctx))
    const issues = nested.safeParse({ shipping: { ...berlin, phone: '030' } }).error?.issues
    expect(issues?.map((i) => i.path.join('.'))).toEqual(['shipping.phone'])
  })
})
