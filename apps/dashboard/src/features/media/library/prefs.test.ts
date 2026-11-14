import { afterEach, describe, expect, it } from 'vitest'
import { readPrefs, savePrefs } from './prefs'

afterEach(() => localStorage.clear())

describe('media library view preferences', () => {
  it('starts on the gallery, newest first', () => {
    expect(readPrefs()).toEqual({ view: 'gallery', rowHeight: 190, sort: 'NEWEST' })
  })

  it('reads back what was saved', () => {
    savePrefs({ view: 'list', rowHeight: 240, sort: 'NAME' })
    expect(readPrefs()).toEqual({ view: 'list', rowHeight: 240, sort: 'NAME' })
  })

  it('falls back field by field on values it does not know', () => {
    localStorage.setItem('media-library:view', JSON.stringify({ view: 'mosaic', rowHeight: 9000, sort: 'RANDOM' }))
    expect(readPrefs()).toEqual({ view: 'gallery', rowHeight: 190, sort: 'NEWEST' })
    localStorage.setItem('media-library:view', 'not json')
    expect(readPrefs().view).toBe('gallery')
  })
})
