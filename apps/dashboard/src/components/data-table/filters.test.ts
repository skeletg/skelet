import { describe, expect, it } from 'vitest'
import { booleanFilterFn, dateRangeFilterFn, numberRangeFilterFn } from './filterFns'

// The matchers decide what a client-side list shows, so the boundaries are the whole contract
type Fn = (row: { getValue: (id: string) => unknown }, id: string, value: unknown) => boolean

const run = (fn: unknown, value: unknown, cell: unknown) =>
  (fn as Fn)({ getValue: () => cell }, 'c', value)

describe('numberRangeFilterFn', () => {
  it('treats both bounds as inclusive', () => {
    expect(run(numberRangeFilterFn, { min: 10, max: 20 }, 10)).toBe(true)
    expect(run(numberRangeFilterFn, { min: 10, max: 20 }, 20)).toBe(true)
    expect(run(numberRangeFilterFn, { min: 10, max: 20 }, 9)).toBe(false)
    expect(run(numberRangeFilterFn, { min: 10, max: 20 }, 21)).toBe(false)
  })

  it('accepts a half-open range', () => {
    expect(run(numberRangeFilterFn, { min: 10 }, 1000)).toBe(true)
    expect(run(numberRangeFilterFn, { max: 10 }, 1000)).toBe(false)
  })

  it('coerces numeric strings, since a cell can hold cents as text', () => {
    expect(run(numberRangeFilterFn, { min: 5 }, '10')).toBe(true)
  })

  it('drops rows whose value is not a number rather than showing them', () => {
    expect(run(numberRangeFilterFn, { min: 5 }, 'abc')).toBe(false)
  })
})

describe('dateRangeFilterFn', () => {
  // Picking a single day must match that day - an exclusive `to` matched nothing, which reads as broken
  it('includes the whole of the `to` day', () => {
    const v = { from: '2026-08-09', to: '2026-08-09' }
    expect(run(dateRangeFilterFn, v, '2026-08-09T00:00:00Z')).toBe(true)
    expect(run(dateRangeFilterFn, v, '2026-08-09T23:30:00')).toBe(true)
    expect(run(dateRangeFilterFn, v, '2026-08-10T00:30:00')).toBe(false)
  })

  it('accepts an open end', () => {
    expect(run(dateRangeFilterFn, { from: '2026-01-01' }, '2030-01-01')).toBe(true)
    expect(run(dateRangeFilterFn, { to: '2026-01-01' }, '2030-01-01')).toBe(false)
  })

  it('drops unparseable dates', () => {
    expect(run(dateRangeFilterFn, { from: '2026-01-01' }, 'not-a-date')).toBe(false)
  })
})

describe('booleanFilterFn', () => {
  it('matches truthiness both ways', () => {
    expect(run(booleanFilterFn, true, true)).toBe(true)
    expect(run(booleanFilterFn, false, false)).toBe(true)
    expect(run(booleanFilterFn, true, false)).toBe(false)
  })

  // An unset filter must not hide rows
  it('passes everything when no choice is made', () => {
    expect(run(booleanFilterFn, undefined, false)).toBe(true)
  })
})
