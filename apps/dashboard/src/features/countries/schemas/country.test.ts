import { describe, expect, it } from 'vitest'
import { countryDiff, countrySchema, toCountryInput, toCountryValues } from './country'

const stored = {
  code: 'DE',
  name: 'Germany',
  nativeName: 'Deutschland',
  emojiFlag: '🇩🇪',
  phoneCode: '+49',
  iso3Code: 'DEU',
  numericCode: '276',
  defaultCurrencyCode: 'EUR',
  isActive: true,
  sortOrder: 0,
  metadata: [{ key: 'vat', value: 'DE123' }],
  regions: [],
}

describe('country schema', () => {
  it('accepts a stored record unchanged', () => {
    expect(countrySchema.safeParse(toCountryValues(stored)).success).toBe(true)
  })

  it('wants an exact alpha-2 code and a name', () => {
    const issues = countrySchema
      .safeParse({ ...toCountryValues(), code: 'DEU' })
      .error?.issues.map((i) => i.path.join('.'))
    expect(issues).toEqual(['code', 'name'])
  })

  it('diffs metadata alongside the fields', () => {
    const before = toCountryValues(stored)
    const after = { ...before, phoneCode: '', metadata: [{ key: 'vat', value: 'DE999' }] }
    expect(countryDiff(before, after)).toEqual([
      { kind: 'change', label: 'Phone code', before: '+49', after: '' },
      { kind: 'change', label: 'Metadata · vat', before: 'DE123', after: 'DE999' },
    ])
  })

  it('sends a cleared field as empty rather than leaving it out', () => {
    const input = toCountryInput({ ...toCountryValues(stored), nativeName: '', defaultCurrencyCode: '' })
    expect(input.nativeName).toBe('')
    expect(input.defaultCurrencyCode).toBe('')
  })
})
