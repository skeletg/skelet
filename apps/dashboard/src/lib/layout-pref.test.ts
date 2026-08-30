import { describe, expect, it } from 'vitest'
import {
  EMPTY_LAYOUT,
  applyLayout,
  isHiddenInLayout,
  moveInLayout,
  toggleInLayout,
} from './layout-pref'

const ITEMS = [{ id: 'a' }, { id: 'b' }, { id: 'c' }]

describe('applyLayout', () => {
  it('leaves the default order alone', () => {
    expect(applyLayout(ITEMS, EMPTY_LAYOUT)).toEqual(ITEMS)
  })

  it('follows a stored order', () => {
    expect(applyLayout(ITEMS, { order: ['c', 'a', 'b'], hidden: [] }).map((i) => i.id)).toEqual([
      'c',
      'a',
      'b',
    ])
  })

  // a widget added after the admin arranged theirs must not vanish, and a removed one must not crash
  it('drops unknown ids and appends new items in default position', () => {
    const out = applyLayout(ITEMS, { order: ['c', 'gone', 'a'], hidden: [] })
    expect(out.map((i) => i.id)).toEqual(['c', 'a', 'b'])
  })
})

describe('moveInLayout', () => {
  it('moves against the list on screen, not the stored order', () => {
    const next = moveInLayout(EMPTY_LAYOUT, ['a', 'b', 'c'], 'c', -1)
    expect(next.order).toEqual(['a', 'c', 'b'])
  })

  it('refuses to move past either end', () => {
    expect(moveInLayout(EMPTY_LAYOUT, ['a', 'b'], 'a', -1)).toEqual(EMPTY_LAYOUT)
    expect(moveInLayout(EMPTY_LAYOUT, ['a', 'b'], 'b', 1)).toEqual(EMPTY_LAYOUT)
  })

  it('ignores an id that is not on screen', () => {
    expect(moveInLayout(EMPTY_LAYOUT, ['a', 'b'], 'z', 1)).toEqual(EMPTY_LAYOUT)
  })
})

describe('isHiddenInLayout', () => {
  it('a normal item is visible until it lands in hidden', () => {
    expect(isHiddenInLayout(EMPTY_LAYOUT, 'a')).toBe(false)
    expect(isHiddenInLayout({ order: [], hidden: ['a'] }, 'a')).toBe(true)
  })

  // an opt-in Home section has the opposite default - absent from both lists means off
  it('a default-hidden item is hidden until it lands in shown', () => {
    expect(isHiddenInLayout(EMPTY_LAYOUT, 'a', true)).toBe(true)
    expect(isHiddenInLayout({ order: [], hidden: [], shown: ['a'] }, 'a', true)).toBe(false)
  })

  // hidden wins, so switching a section off after switching it on actually turns it off
  it('an explicit hide beats an explicit show', () => {
    expect(isHiddenInLayout({ order: [], hidden: ['a'], shown: ['a'] }, 'a', true)).toBe(true)
  })
})

describe('toggleInLayout', () => {
  it('hides then shows again, leaving the order untouched', () => {
    const hidden = toggleInLayout({ order: ['b', 'a'], hidden: [] }, 'a')
    expect(hidden).toEqual({ order: ['b', 'a'], hidden: ['a'], shown: [] })
    expect(isHiddenInLayout(hidden, 'a')).toBe(true)
    const back = toggleInLayout(hidden, 'a')
    expect(back.hidden).toEqual([])
    expect(isHiddenInLayout(back, 'a')).toBe(false)
  })

  it('round-trips a default-hidden item through both lists', () => {
    const on = toggleInLayout(EMPTY_LAYOUT, 'traffic', true)
    expect(on.shown).toEqual(['traffic'])
    expect(isHiddenInLayout(on, 'traffic', true)).toBe(false)
    const off = toggleInLayout(on, 'traffic', true)
    expect(off.shown).toEqual([])
    expect(isHiddenInLayout(off, 'traffic', true)).toBe(true)
  })

  // an older stored layout has no `shown` key at all, and toggling must not choke on it
  it('tolerates a layout stored before the shown list existed', () => {
    const withoutShown = { order: ['a'], hidden: ['a'] }
    expect(toggleInLayout(withoutShown, 'a')).toEqual({ order: ['a'], hidden: [], shown: ['a'] })
  })

  it('never lists the same id twice', () => {
    const twice = toggleInLayout(toggleInLayout(EMPTY_LAYOUT, 'a'), 'b')
    expect(twice.hidden).toEqual(['a', 'b'])
  })
})
