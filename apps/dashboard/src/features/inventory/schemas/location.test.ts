import { describe, expect, it } from 'vitest'
import { locationDiff, locationSchema, toLocationInput, toLocationValues, type ExistingLocation } from './location'

// What these pin: the schema accepts what the server sends back, the diff lists hours and metadata
// the way the confirm dialog shows them, and the mutation input converts coordinates and hours

const stored: ExistingLocation = {
  id: 'loc-1',
  slug: 'main',
  name: 'Main',
  isActive: true,
  storefrontVisible: false,
  type: 'WAREHOUSE',
  line1: '',
  line2: '',
  city: 'Berlin',
  region: '',
  postalCode: '',
  countryCode: 'DE',
  latitude: 52.52,
  longitude: null,
  timezone: 'Europe/Berlin',
  hours: { monday: { closed: false, open: '09:00', close: '17:00' } },
  email: '',
  phone: '',
  contactName: '',
  externalReference: '',
  pickupEnabled: false,
  pickupInstructions: '',
  metadata: [{ key: 'carrier', value: 'dhl' }],
}

describe('location schema', () => {
  it("reads the phone in the location's own country", () => {
    const berlin = { ...toLocationValues(stored), phone: '030 1234567' }
    expect(locationSchema.safeParse(berlin).success).toBe(true)
    const issues = locationSchema.safeParse({ ...berlin, countryCode: 'US' }).error?.issues
    expect(issues?.map((i) => i.path.join('.'))).toEqual(['phone'])
    expect(issues?.[0].message).toMatch(/for US/)
  })

  it('accepts a stored record unchanged', () => {
    expect(locationSchema.safeParse(toLocationValues(stored)).success).toBe(true)
  })

  it('rejects out-of-range coordinates and bad emails', () => {
    const values = { ...toLocationValues(stored), latitude: '91', email: 'nope' }
    const issues = locationSchema.safeParse(values).error?.issues.map((i) => i.path.join('.'))
    expect(issues).toEqual(['latitude', 'email'])
  })

  it('diffs hours per day and metadata per key', () => {
    const before = toLocationValues(stored)
    const after = {
      ...before,
      city: 'Hamburg',
      hours: { ...before.hours, monday: { ...before.hours.monday, closed: true } },
      metadata: [{ key: 'account', value: '42' }],
    }
    expect(locationDiff(before, after)).toEqual([
      { kind: 'change', label: 'City', before: 'Berlin', after: 'Hamburg' },
      { kind: 'change', label: 'Hours · Mon', before: '09:00–17:00', after: 'closed' },
      { kind: 'add', label: 'Metadata · account', before: '', after: '42' },
      { kind: 'remove', label: 'Metadata · carrier', before: 'dhl', after: '' },
    ])
  })

  it('builds the mutation input', () => {
    const input = toLocationInput({ ...toLocationValues(stored), metadata: [{ key: ' ', value: 'x' }] })
    expect(input.latitude).toBe(52.52)
    expect(input.longitude).toBeNull()
    expect(input.hours.monday).toEqual({ closed: false, open: '09:00', close: '17:00' })
    expect(input.hours.tuesday).toEqual({ closed: true })
    expect(input.metadata).toEqual([])
  })
})


describe('public storefront visibility', () => {
  it('defaults to hidden and includes visibility in the mutation and confirmation diff', () => {
    const before = { ...toLocationValues(), type: 'RETAIL_STORE' as const }
    expect(before.storefrontVisible).toBe(false)
    const after = { ...before, storefrontVisible: true }
    expect(toLocationInput(after).storefrontVisible).toBe(true)
    expect(locationDiff(before, after)).toEqual([
      { kind: 'change', label: 'List in store finder', before: 'no', after: 'yes' },
    ])
    expect(toLocationValues({ ...stored, storefrontVisible: true }).storefrontVisible).toBe(true)
  })
})


describe('location classification and customer access', () => {
  it('keeps the API type and rejects arbitrary purposes', () => {
    expect(toLocationValues(stored).type).toBe('WAREHOUSE')
    expect(toLocationValues().type).toBe('UNCLASSIFIED')
    expect(locationSchema.safeParse({ ...toLocationValues(stored), type: 'custom' }).success).toBe(false)
  })
  it('reviews and submits removal of both services when converting a retail store', () => {
    const before = { ...toLocationValues(stored), type: 'RETAIL_STORE' as const, storefrontVisible: true, pickupEnabled: true }
    const after = { ...before, type: 'WAREHOUSE' as const }
    expect(locationDiff(before, after)).toEqual(expect.arrayContaining([
      { kind: 'change', label: 'Type', before: 'Retail store', after: 'Warehouse' },
      { kind: 'change', label: 'List in store finder', before: 'yes', after: 'no' },
      { kind: 'change', label: 'Accept online orders for pickup', before: 'yes', after: 'no' },
    ]))
    expect(toLocationInput(after)).toMatchObject({ type: 'WAREHOUSE', storefrontVisible: false, pickupEnabled: false })
  })
  it('keeps a walk-in retail store visible without online pickup', () => {
    expect(toLocationInput({ ...toLocationValues(stored), type: 'RETAIL_STORE', storefrontVisible: true, pickupEnabled: false })).toMatchObject({ storefrontVisible: true, pickupEnabled: false })
  })
})
