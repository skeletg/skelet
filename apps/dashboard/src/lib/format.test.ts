import { afterEach, describe, expect, it, vi } from 'vitest'

import {
  dayEndISO,
  dayStartISO,
  formatCents,
  fromMinor,
  toMinor,
  formatDate,
  formatDateTime,
  formatNumber,
  formatShortDay,
  formatTimestamp,
  timeAgo,
} from './format'

// Baseline smoke tests for the money formatter - without a registry hint the Intl fallback formats by code
describe('formatCents', () => {
  it('formats whole dollars', () => {
    expect(formatCents(9900, 'USD')).toMatch(/99/)
  })

  it('keeps cents precision', () => {
    expect(formatCents(7901, 'USD')).toMatch(/79[.,]01/)
  })

  it('handles negative amounts (ledger debits)', () => {
    expect(formatCents(-5000, 'USD')).toMatch(/50/)
    expect(formatCents(-5000, 'USD')).toMatch(/-/)
  })

  it('never throws on unknown currency codes', () => {
    expect(() => formatCents(1234, 'XXX')).not.toThrow()
  })

  it('does not divide zero-decimal currencies by 100', () => {
    expect(formatCents(1250, 'JPY')).toMatch(/1[,.]?250/)
  })
})

describe('toMinor / fromMinor', () => {
  it('converts by the currency decimals', () => {
    expect(toMinor('12.50', 'USD')).toBe(1250)
    expect(toMinor('0.29', 'USD')).toBe(29)
    expect(toMinor('1250', 'JPY')).toBe(1250)
    expect(toMinor('1.234', 'BHD')).toBe(1234)
  })

  it('refuses blank, junk and extra decimals instead of rounding', () => {
    expect(toMinor('', 'USD')).toBeNull()
    expect(toMinor('abc', 'USD')).toBeNull()
    expect(toMinor('1.234', 'USD')).toBeNull()
    expect(toMinor('12.5', 'JPY')).toBeNull()
  })

  it('round-trips', () => {
    expect(fromMinor(1250, 'USD')).toBe('12.50')
    expect(fromMinor(1250, 'JPY')).toBe('1250')
    expect(toMinor(fromMinor(1999, 'EUR'), 'EUR')).toBe(1999)
  })
})

// The date helpers replaced a dozen local copies - the expectations are built with the same Intl options, so they
// hold in whatever locale and timezone the runner has
const ISO = '2026-03-04T15:15:00Z'

describe('formatDateTime', () => {
  it('renders a medium date and short time', () => {
    expect(formatDateTime(ISO)).toBe(new Date(ISO).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' }))
  })

  it('renders a dash for missing or unparseable values, never 1970 or Invalid Date', () => {
    expect(formatDateTime(null)).toBe('-')
    expect(formatDateTime(undefined)).toBe('-')
    expect(formatDateTime('')).toBe('-')
    expect(formatDateTime('not a date')).toBe('-')
  })
})

describe('formatDate', () => {
  it('renders a medium date without the time', () => {
    expect(formatDate(ISO)).toBe(new Date(ISO).toLocaleDateString(undefined, { dateStyle: 'medium' }))
  })

  it('renders a dash for missing or unparseable values', () => {
    expect(formatDate(null)).toBe('-')
    expect(formatDate('garbage')).toBe('-')
  })
})

describe('formatShortDay', () => {
  it('renders month and day', () => {
    expect(formatShortDay(ISO)).toBe(new Date(ISO).toLocaleDateString(undefined, { month: 'short', day: 'numeric' }))
  })

  it('falls back to the raw value so a non-date bucket key still reads', () => {
    expect(formatShortDay('other')).toBe('other')
  })
})

describe('formatTimestamp', () => {
  it('uses the browser default rendering, seconds included', () => {
    expect(formatTimestamp(ISO)).toBe(new Date(ISO).toLocaleString())
  })

  it('renders a dash for a missing value', () => {
    expect(formatTimestamp(null)).toBe('-')
    expect(formatTimestamp(undefined)).toBe('-')
    expect(formatTimestamp('')).toBe('-')
  })
})

describe('formatNumber', () => {
  it('groups thousands in the runner locale', () => {
    expect(formatNumber(1234567)).toBe((1234567).toLocaleString())
    expect(formatNumber(0)).toBe('0')
  })
})

describe('timeAgo', () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  const at = (iso: string) => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date(iso))
  }

  it('says just now under a minute', () => {
    at('2026-03-04T15:15:30Z')
    expect(timeAgo(ISO)).toBe('just now')
  })

  it('steps through minutes, hours and days', () => {
    at('2026-03-04T15:20:00Z')
    expect(timeAgo(ISO)).toBe('5m ago')
    at('2026-03-04T18:15:00Z')
    expect(timeAgo(ISO)).toBe('3h ago')
    at('2026-03-06T15:15:00Z')
    expect(timeAgo(ISO)).toBe('2d ago')
  })

  it('clamps a timestamp from the future to just now', () => {
    at('2026-03-04T15:00:00Z')
    expect(timeAgo(ISO)).toBe('just now')
  })

  it('returns empty for an unparseable value', () => {
    expect(timeAgo('nope')).toBe('')
  })
})

// Run with TZ set west and east of UTC too - in UTC the old new Date('YYYY-MM-DD') happened to be right
describe('dayStartISO / dayEndISO', () => {
  it('bound the picked day in local time', () => {
    const start = new Date(dayStartISO('2026-03-04'))
    const end = new Date(dayEndISO('2026-03-04'))
    expect([start.getFullYear(), start.getMonth(), start.getDate(), start.getHours()]).toEqual([2026, 2, 4, 0])
    expect([end.getDate(), end.getHours(), end.getMinutes()]).toEqual([4, 23, 59])
  })

  it('a one-day range spans the whole day', () => {
    expect(new Date(dayEndISO('2026-03-04')).getTime() - new Date(dayStartISO('2026-03-04')).getTime()).toBe(86_399_999)
  })
})
