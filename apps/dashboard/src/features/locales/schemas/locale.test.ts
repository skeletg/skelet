import { describe, expect, it } from 'vitest'
import { localeDiff, localeFieldErrors, localeSchema, toLocaleInput, toLocaleValues } from './locale'

describe('locale schema', () => {
  it('accepts a fresh create once code and name are set', () => {
    expect(localeSchema.safeParse({ ...toLocaleValues(), code: 'es-mx', name: 'Spanish (Mexico)' }).success).toBe(true)
  })

  it('rejects a malformed code and a non-ISO country', () => {
    const issues = localeSchema
      .safeParse({ ...toLocaleValues(), code: 'ES_MX', name: 'x', defaultCountryCode: 'MEX' })
      .error?.issues.map((i) => i.path.join('.'))
    expect(issues).toEqual(['code', 'defaultCountryCode'])
  })

  it('shows the first day of week by name in the diff', () => {
    const before = toLocaleValues()
    expect(localeDiff(before, { ...before, firstDayOfWeek: 0 })).toEqual([
      { kind: 'change', label: 'First day of week', before: 'Monday', after: 'Sunday' },
    ])
  })

  it('leaves an empty native name out so the service fills it from Name', () => {
    expect(toLocaleInput({ ...toLocaleValues(), name: 'German' }).nativeName).toBeUndefined()
  })

  it('routes service messages to their field', () => {
    expect(localeFieldErrors('hreflang_value does not look like BCP 47')).toEqual({
      hreflangValue: "Doesn't look like a BCP 47 code.",
    })
    expect(localeFieldErrors('boom')).toEqual({})
  })
})
