import { describe, expect, it } from 'vitest'
import { toCsv } from './exportRows'

// A grid export lands in Excel, so the quoting rules are the contract - a stray comma or newline
// silently shifts every following column
describe('toCsv', () => {
  it('joins with CRLF, as RFC 4180 and Excel expect', () => {
    expect(toCsv(['a', 'b'], [['1', '2']])).toBe('a,b\r\n1,2')
  })

  it('quotes fields containing a delimiter', () => {
    expect(toCsv(['name'], [['Acme, Inc']])).toBe('name\r\n"Acme, Inc"')
  })

  it('doubles inner quotes rather than escaping them', () => {
    expect(toCsv(['name'], [['He said "hi"']])).toBe('name\r\n"He said ""hi"""')
  })

  it('quotes fields containing a newline', () => {
    expect(toCsv(['note'], [['line1\nline2']])).toBe('note\r\n"line1\nline2"')
  })

  it('leaves ordinary values unquoted', () => {
    expect(toCsv(['sku'], [['BENCH-000001']])).toBe('sku\r\nBENCH-000001')
  })

  it('emits a header-only file for an empty result', () => {
    expect(toCsv(['a', 'b'], [])).toBe('a,b')
  })
})
