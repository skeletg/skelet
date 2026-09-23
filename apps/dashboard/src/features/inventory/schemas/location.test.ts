import { describe, expect, it } from 'vitest'
import { locationDiff, locationSchema, toLocationInput, toLocationValues } from './location'

// What these pin: the schema accepts what the server sends back, the diff lists hours and metadata
// the way the confirm dialog shows them, and the mutation input converts coordinates and hours

const stored = {
  id: 'loc-1',
  slug: 'main',
  name: 'Main',
  isActive: true,
  type: 'warehouse',
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
