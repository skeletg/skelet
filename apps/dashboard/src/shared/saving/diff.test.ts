import { describe, expect, it } from 'vitest'
import { added, changed, display, fieldDiff, removed } from './diff'
import { normalizeDiff } from './saveRegistry'

describe('display', () => {
  it('canonicalizes empty-ish values to the empty string', () => {
    expect(display(null)).toBe('')
    expect(display(undefined)).toBe('')
    expect(display('')).toBe('')
    expect(display('   ')).toBe('')
  })
  it('renders booleans as yes/no and numbers as strings', () => {
    expect(display(true)).toBe('yes')
    expect(display(false)).toBe('no')
    expect(display(42)).toBe('42')
  })
  it('joins arrays and clips long strings', () => {
    expect(display(['a', 'b'])).toBe('a, b')
    expect(display('x'.repeat(200)).length).toBeLessThanOrEqual(120)
    expect(display('x'.repeat(200)).endsWith('…')).toBe(true)
  })
})

describe('changed', () => {
  it('returns null when normalized values are equal', () => {
    expect(changed('Name', 'a ', 'a')).toBeNull()
    expect(changed('Price', 5, '5')).toBeNull()
    expect(changed('Flag', null, '')).toBeNull()
  })
  it('returns a change entry for real edits', () => {
    expect(changed('Name', 'a', 'b')).toEqual({ kind: 'change', label: 'Name', before: 'a', after: 'b' })
  })
})

describe('added / removed', () => {
  it('carry their kinds and summaries', () => {
    expect(added('Value · Deluxe', 'fixed 900')).toEqual({ kind: 'add', label: 'Value · Deluxe', before: '', after: 'fixed 900' })
    expect(removed('Option · Gift wrap')).toEqual({ kind: 'remove', label: 'Option · Gift wrap', before: '', after: '' })
  })
})

describe('fieldDiff', () => {
  type Form = { name: string; qty: number | null; on: boolean }
  const fields = [
    { label: 'Name', value: (f: Form) => f.name },
    { label: 'Qty', value: (f: Form) => f.qty },
    { label: 'Enabled', value: (f: Form) => f.on },
  ]
  it('emits entries only for fields that really differ', () => {
    const orig: Form = { name: 'Door', qty: null, on: false }
    const same: Form = { name: ' Door ', qty: null, on: false }
    expect(fieldDiff(orig, same, fields)).toEqual([])
    const edited: Form = { name: 'Door', qty: 4, on: true }
    expect(fieldDiff(orig, edited, fields)).toEqual([
      { kind: 'change', label: 'Qty', before: '', after: '4' },
      { kind: 'change', label: 'Enabled', before: 'no', after: 'yes' },
    ])
  })
})

describe('normalizeDiff (central sanitizer)', () => {
  it('drops no-op change entries but keeps adds/removes and real changes', () => {
    const kept = normalizeDiff('s', [
      { label: 'noop', before: 'x', after: 'x' },
      { label: 'real', before: 'x', after: 'y' },
      { kind: 'add', label: 'new row', before: '', after: '' },
      { kind: 'remove', label: 'old row', before: '', after: '' },
    ])
    expect(kept.map((d) => d.label)).toEqual(['real', 'new row', 'old row'])
  })
})
