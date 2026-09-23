import { describe, expect, it } from 'vitest'
import { methodSchema, toMethodInput, toMethodValues } from './method'
import { toZoneInput, toZoneValues, zoneDiff, zoneSchema } from './zone'

const zone = { id: 'z1', channelId: 'c1', code: 'us', name: 'United States', sortOrder: 0, isActive: true, countries: ['US', 'CA'] }

describe('zone schema', () => {
  it('accepts a stored zone unchanged', () => {
    expect(zoneSchema.safeParse(toZoneValues(zone)).success).toBe(true)
  })

  it('rejects anything but 2-letter codes', () => {
    expect(zoneSchema.safeParse({ ...toZoneValues(zone), countries: 'US, USA' }).success).toBe(false)
  })

  it('treats a reformatted but equal country list as no change', () => {
    const before = toZoneValues(zone)
    expect(zoneDiff(before, { ...before, countries: 'us,ca' })).toEqual([])
    expect(zoneDiff(before, { ...before, countries: 'US, CA, MX' })).toEqual([
      { kind: 'change', label: 'Countries', before: 'US, CA', after: 'US, CA, MX' },
    ])
  })

  it('uppercases and de-duplicates countries on the way out', () => {
    expect(toZoneInput({ ...toZoneValues(zone), countries: 'us, ca, US' }).countries).toEqual(['US', 'CA'])
  })
})

describe('method schema', () => {
  const method = {
    id: 'm1',
    code: 'std',
    name: 'Standard',
    calcType: 'FREE_ABOVE' as const,
    flatCents: 500,
    freeAboveCents: 5000,
    sortOrder: 0,
    isActive: true,
    countryMode: 'all',
    allowedCountries: [],
  }

  it('accepts a stored method unchanged and round-trips cents', () => {
    const values = toMethodValues(method)
    expect(methodSchema.safeParse(values).success).toBe(true)
    expect(toMethodInput(values)).toMatchObject({ flatCents: 500, freeAboveCents: 5000, allowedCountries: [] })
  })

  it('rejects a negative price', () => {
    expect(methodSchema.safeParse({ ...toMethodValues(method), flatDollars: '-1' }).success).toBe(false)
  })

  it('only sends allowed countries in specific mode', () => {
    const values = { ...toMethodValues(method), allowedCountries: 'us' }
    expect(toMethodInput(values).allowedCountries).toEqual([])
    expect(toMethodInput({ ...values, countryMode: 'specific' }).allowedCountries).toEqual(['US'])
  })
})
