import { describe, expect, it } from 'vitest'
import { added, changed, clip, display, fieldDiff, removed, sameJson, sameSet } from './diff'
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

describe('clip', () => {
  it('leaves short strings alone', () => {
    expect(clip('short')).toBe('short')
    expect(clip('x'.repeat(120))).toBe('x'.repeat(120))
  })

  it('cuts long strings to 120 characters, ellipsis included', () => {
    const out = clip('x'.repeat(200))
    expect(out).toHaveLength(120)
    expect(out.endsWith('…')).toBe(true)
  })
})

describe('sameJson', () => {
  it('compares structurally', () => {
    expect(sameJson({ a: [1, 2] }, { a: [1, 2] })).toBe(true)
    expect(sameJson({ a: [1, 2] }, { a: [2, 1] })).toBe(false)
  })

  it('treats null and undefined as equal, so an unset draft field is not a change', () => {
    expect(sameJson(null, undefined)).toBe(true)
    expect(sameJson(undefined, {})).toBe(false)
  })
})

describe('sameSet', () => {
  it('ignores order', () => {
    expect(sameSet(['a', 'b', 'c'], ['c', 'a', 'b'])).toBe(true)
  })

  it('sees additions and removals', () => {
    expect(sameSet(['a', 'b'], ['a'])).toBe(false)
    expect(sameSet(['a'], ['a', 'b'])).toBe(false)
    expect(sameSet(['a', 'b'], ['a', 'c'])).toBe(false)
  })

  it('treats two empty lists as the same', () => {
    expect(sameSet([], [])).toBe(true)
  })
})
