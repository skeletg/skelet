import { describe, expect, it } from 'vitest'
import { z } from 'zod'

import {
  absoluteUrl,
  contentSlug,
  countryCode,
  currencyCode,
  dateRangeIssues,
  dollarsToCents,
  email,
  handleSlug,
  isNumberText,
  isoToLocalInput,
  localInputToISO,
  money,
  newPassword,
  numberText,
  numberTextMessage,
  optionalAbsoluteUrl,
  optionalEmail,
  parseNumberText,
  wholeNumber,
} from './fields'

const message = (schema: { safeParse: (v: unknown) => { success: boolean; error?: { issues: { message: string }[] } } }, v: unknown) =>
  schema.safeParse(v).error?.issues[0]?.message

describe('email', () => {
  it('accepts a padded address and refuses blank or malformed input', () => {
    expect(email().safeParse('  a@b.co ').success).toBe(true)
    expect(message(email(), '')).toBe('Enter a valid email address')
    expect(message(email('Enter the order email'), 'nope')).toBe('Enter the order email')
  })

  it('optionalEmail lets blank through', () => {
    expect(optionalEmail().safeParse('   ').success).toBe(true)
    expect(optionalEmail().safeParse('a@b.co').success).toBe(true)
    expect(optionalEmail().safeParse('a@').success).toBe(false)
  })
})

describe('newPassword', () => {
  it('holds the backend floor of 8 characters', () => {
    expect(message(newPassword(), '1234567')).toBe('At least 8 characters')
    expect(newPassword().safeParse('12345678').success).toBe(true)
  })
})

describe('parseNumberText', () => {
  it('reads trimmed numbers and nulls blank or junk', () => {
    expect(parseNumberText(' 12.5 ')).toBe(12.5)
    expect(parseNumberText('')).toBeNull()
    expect(parseNumberText('  ')).toBeNull()
    expect(parseNumberText('abc')).toBeNull()
    expect(parseNumberText('Infinity')).toBeNull()
  })
})

describe('isNumberText', () => {
  it('refuses blank unless optional', () => {
    expect(isNumberText('')).toBe(false)
    expect(isNumberText(' ', { optional: true })).toBe(true)
  })

  it('applies int and the bounds', () => {
    expect(isNumberText('1.5', { int: true })).toBe(false)
    expect(isNumberText('-1', { min: 0 })).toBe(false)
    expect(isNumberText('0', { min: 0 })).toBe(true)
    expect(isNumberText('0', { above: 0 })).toBe(false)
    expect(isNumberText('30001', { max: 30000 })).toBe(false)
  })
})

describe('numberTextMessage', () => {
  it('says the rule in words', () => {
    expect(numberTextMessage({})).toBe('A number')
    expect(numberTextMessage({ int: true })).toBe('A whole number')
    expect(numberTextMessage({ min: 0 })).toBe('Zero or more')
    expect(numberTextMessage({ int: true, min: 0 })).toBe('A whole number, zero or more')
    expect(numberTextMessage({ int: true, min: 1 })).toBe('A whole number, one or more')
    expect(numberTextMessage({ above: 0 })).toBe('Above 0')
    expect(numberTextMessage({ int: true, min: 0, max: 30000 })).toBe('A whole number from 0 to 30000')
    expect(numberTextMessage({ min: -90, max: 90 })).toBe('Between -90 and 90')
  })

  it('numberText uses it unless given a message', () => {
    expect(message(numberText({ min: 0 }), '-1')).toBe('Zero or more')
    expect(message(numberText({ int: true }, 'Whole grams'), '1.5')).toBe('Whole grams')
  })
})

describe('money', () => {
  it('is zero or more, blank only when optional', () => {
    expect(message(money(), '')).toBe('Zero or more')
    expect(money({ optional: true }).safeParse('').success).toBe(true)
    expect(money().safeParse('12.50').success).toBe(true)
  })

  it('converts major units to cents', () => {
    expect(dollarsToCents('12.5')).toBe(1250)
    expect(dollarsToCents('')).toBe(0)
    expect(dollarsToCents('abc')).toBe(0)
  })
})

describe('wholeNumber', () => {
  it('refuses fractions', () => {
    expect(message(wholeNumber(), 1.5)).toBe('Whole numbers only')
    expect(wholeNumber().safeParse(3).success).toBe(true)
  })
})

describe('slugs', () => {
  it('content slugs allow dots, handles do not', () => {
    expect(contentSlug().safeParse('summer.sale-2').success).toBe(true)
    expect(message(contentSlug(), 'Summer')).toBe('Lowercase letters, numbers, dots, underscores and hyphens only')
    expect(handleSlug().safeParse('main_menu-1').success).toBe(true)
    expect(message(handleSlug(), 'main.menu')).toBe('Lowercase letters, numbers, underscores and hyphens only')
    expect(message(handleSlug(), '  ')).toBe('Slug is required')
  })
})

describe('URLs', () => {
  it('absoluteUrl needs a full URL, optionalAbsoluteUrl also takes blank', () => {
    expect(absoluteUrl().safeParse('https://example.com/hook').success).toBe(true)
    expect(message(absoluteUrl(), 'example.com')).toBe('Enter a full URL, starting with https://')
    expect(optionalAbsoluteUrl().safeParse('').success).toBe(true)
    expect(optionalAbsoluteUrl().safeParse('nope').success).toBe(false)
  })
})

describe('ISO codes', () => {
  it('holds the letter counts and case', () => {
    expect(countryCode().safeParse('US').success).toBe(true)
    expect(countryCode().safeParse('us').success).toBe(false)
    expect(countryCode().safeParse('').success).toBe(false)
    expect(countryCode({ optional: true }).safeParse('').success).toBe(true)
    expect(currencyCode().safeParse('EUR').success).toBe(true)
    expect(message(currencyCode('Pick a currency'), '')).toBe('Pick a currency')
  })
})

describe('datetime-local', () => {
  it('round-trips through ISO in local time', () => {
    expect(isoToLocalInput(localInputToISO('2026-05-01T12:30'))).toBe('2026-05-01T12:30')
    expect(isoToLocalInput(null)).toBe('')
    expect(localInputToISO('')).toBeNull()
    expect(localInputToISO('not a date')).toBeNull()
  })

  it('dateRangeIssues refuses an end at or before the start', () => {
    const schema = z.object({ from: z.string(), to: z.string() }).superRefine((v, ctx) => dateRangeIssues(v.from, v.to, ['to'], ctx))
    expect(schema.safeParse({ from: '2026-05-01T12:00', to: '2026-05-01T13:00' }).success).toBe(true)
    expect(schema.safeParse({ from: '2026-05-01T12:00', to: '' }).success).toBe(true)
    expect(message(schema, { from: '2026-05-01T12:00', to: '2026-05-01T12:00' })).toBe('Ends after it starts')
  })
})
