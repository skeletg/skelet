import { describe, expect, it } from 'vitest'
import { formatAsTyped, formatPhone, phoneIssue } from './phone'

describe('phoneIssue', () => {
  it('reads a national number in the given country', () => {
    expect(phoneIssue('01512 3456789', 'DE')).toBeNull()
    expect(phoneIssue('01512 3456789', 'de')).toBeNull()
    expect(phoneIssue('01512 3456789', 'US')).toMatch(/for US/)
  })

  it('judges a +number on its own, whatever the country', () => {
    expect(phoneIssue('+33 6 12 34 56 78', 'DE')).toBeNull()
    expect(phoneIssue('+49 151', 'DE')).not.toBeNull()
  })

  it("leaves a national number with no country to the backend, and blank alone", () => {
    expect(phoneIssue('01512 3456789')).toBeNull()
    expect(phoneIssue('+1 555 000')).toBe('Not a valid phone number')
    expect(phoneIssue('   ', 'DE')).toBeNull()
  })
})

describe('formatting', () => {
  it('formats as typed in the country', () => {
    expect(formatAsTyped('2015550123', 'US')).toBe('(201) 555-0123')
    expect(formatAsTyped('+4915123456789')).toBe('+49 1512 3456789')
  })

  it('shows stored numbers internationally and older free text as it is', () => {
    expect(formatPhone('+4915123456789')).toBe('+49 1512 3456789')
    expect(formatPhone('ask for Max')).toBe('ask for Max')
  })
})
