import { describe, expect, it } from 'vitest'
import type { ConfigFieldData } from '@/features/settings/ConfigField'
import { fieldMatches, labelMatches, searchTerms } from './search'

const field = (over: Partial<ConfigFieldData> = {}): ConfigFieldData => ({
  path: 'checkout/guest/enabled',
  section: 'checkout',
  group: 'guest',
  field: 'enabled',
  label: 'Allow guest checkout',
  type: 'STRING',
  value: 'remote',
  isSet: false,
  isDefault: false,
  public: true,
  scoping: 'GLOBAL',
  scope: { kind: 'DEFAULT' },
  ...over,
})

const labels = { section: 'Checkout', group: 'Guest' }

describe('searchTerms', () => {
  it('splits on any run of whitespace and lowercases', () => {
    expect(searchTerms('  Sales   Guest ')).toEqual(['sales', 'guest'])
  })

  it('treats an empty query as no terms at all', () => {
    expect(searchTerms('   ')).toEqual([])
  })
})

describe('fieldMatches', () => {
  it('keeps everything when nothing was typed', () => {
    expect(fieldMatches(field(), [], labels)).toBe(true)
  })

  it('matches the label', () => {
    expect(fieldMatches(field(), searchTerms('guest'), labels)).toBe(true)
  })

  it('matches the path, so a known config key finds its row', () => {
    expect(fieldMatches(field(), searchTerms('checkout/guest'), labels)).toBe(true)
  })

  it('matches the section and group headings a field sits under', () => {
    expect(fieldMatches(field({ label: 'Enabled' }), searchTerms('checkout'), labels)).toBe(true)
  })

  // the reason values are in the haystack at all: searching "remote" missed fields where it was
  // the VALUE rather than the label
  it('matches the current value', () => {
    expect(fieldMatches(field({ label: 'Provider' }), searchTerms('remote'), labels)).toBe(true)
  })

  it('matches an option label, so you can search by what you would pick', () => {
    const f = field({ label: 'Mode', value: 'a', options: [{ label: 'Warehouse pickup', value: 'a' }] })
    expect(fieldMatches(f, searchTerms('warehouse'), labels)).toBe(true)
  })

  it('never searches a secret’s value, which the server does not send anyway', () => {
    const f = field({ label: 'API key', type: 'SECRET', value: 'super-secret' })
    expect(fieldMatches(f, searchTerms('super-secret'), labels)).toBe(false)
    expect(fieldMatches(f, searchTerms('api'), labels)).toBe(true)
  })

  it('ANDs the terms, so a second word narrows instead of widening', () => {
    expect(fieldMatches(field(), searchTerms('guest checkout'), labels)).toBe(true)
    expect(fieldMatches(field(), searchTerms('guest nonsense'), labels)).toBe(false)
  })
})

describe('labelMatches', () => {
  it('lets a whole group answer for its own name', () => {
    expect(labelMatches('Checkout', searchTerms('check'))).toBe(true)
    expect(labelMatches('Checkout', [])).toBe(true)
  })

  it('ANDs its terms too, so a group only survives if every word is in its name', () => {
    expect(labelMatches('Guest checkout', searchTerms('guest checkout'))).toBe(true)
    expect(labelMatches('Guest checkout', searchTerms('guest payments'))).toBe(false)
  })
})
