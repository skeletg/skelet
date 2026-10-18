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
  businessIdFields: [],
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
    const after = { ...before, nativeName: 'BRD', metadata: [{ key: 'vat', value: 'DE999' }] }
    expect(countryDiff(before, after)).toEqual([
      { kind: 'change', label: 'Native name', before: 'Deutschland', after: 'BRD' },
      { kind: 'change', label: 'Metadata · vat', before: 'DE123', after: 'DE999' },
    ])
  })

  // The phone code follows the country code - a stored typo is replaced, and it never shows as an edit
  it('derives the phone code from the country code', () => {
    const values = { ...toCountryValues(stored), phoneCode: '+4' }
    expect(toCountryInput(values).phoneCode).toBe('+49')
    expect(countryDiff(toCountryValues(stored), values)).toEqual([])
    expect(toCountryInput({ ...values, code: 'XX', phoneCode: '+999' }).phoneCode).toBe('+999')
  })

  it('sends a cleared field as empty rather than leaving it out', () => {
    const input = toCountryInput({ ...toCountryValues(stored), nativeName: '', defaultCurrencyCode: '' })
    expect(input.nativeName).toBe('')
    expect(input.defaultCurrencyCode).toBe('')
  })

  it('stages company ID fields: validated, diffed as one readable line, sent with an empty key left to the server', () => {
    const before = toCountryValues(stored)
    const after = {
      ...before,
      businessIdFields: [
        { key: '', label: 'PIB', required: true, pattern: '\\d{9}', hint: '9 digits' },
        { key: 'mb', label: 'Matični broj', required: false, pattern: '', hint: '' },
      ],
    }
    expect(countrySchema.safeParse(after).success).toBe(true)
    expect(countryDiff(before, after)).toContainEqual({
      kind: 'change',
      label: 'Company ID fields',
      before: expect.anything(),
      after: 'PIB (required, \\d{9}) - 9 digits; Matični broj (key mb)',
    })
    expect(toCountryInput(after).businessIdFields[0]).toEqual({
      key: null,
      label: 'PIB',
      required: true,
      pattern: '\\d{9}',
      hint: '9 digits',
    })
    const bad = { ...after, businessIdFields: [{ key: 'Bad Key', label: '', required: false, pattern: '(', hint: '' }] }
    const paths = countrySchema.safeParse(bad).error?.issues.map((i) => i.path.join('.'))
    expect(paths).toEqual(['businessIdFields.0.key', 'businessIdFields.0.label', 'businessIdFields.0.pattern'])
  })
})
