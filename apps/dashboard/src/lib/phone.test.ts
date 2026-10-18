import { describe, expect, it } from 'vitest'
import { formatAsTyped, formatPhone, phoneIssue } from './phone'

describe('phoneIssue', () => {
  it('reads a national number in the given country', () => {
    expect(phoneIssue('064 123 4567', 'RS')).toBeNull()
    expect(phoneIssue('064 123 4567', 'rs')).toBeNull()
    expect(phoneIssue('064 123 4567', 'US')).toMatch(/for US/)
  })

  it('judges a +number on its own, whatever the country', () => {
    expect(phoneIssue('+49 1512 3456789', 'RS')).toBeNull()
    expect(phoneIssue('+381 64', 'RS')).not.toBeNull()
  })

  it("leaves a national number with no country to the backend, and blank alone", () => {
    expect(phoneIssue('064 123 4567')).toBeNull()
    expect(phoneIssue('+1 555 000')).toBe('Not a valid phone number')
    expect(phoneIssue('   ', 'RS')).toBeNull()
  })
})

describe('formatting', () => {
  it('formats as typed in the country', () => {
    expect(formatAsTyped('2015550123', 'US')).toBe('(201) 555-0123')
    expect(formatAsTyped('+381641234567')).toBe('+381 64 1234567')
  })

  it('shows stored numbers internationally and older free text as it is', () => {
    expect(formatPhone('+381641234567')).toBe('+381 64 1234567')
    expect(formatPhone('ask for Marko')).toBe('ask for Marko')
  })
})
