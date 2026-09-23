import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  DRAFT_TTL_MS,
  clearDraft,
  draftKey,
  purgeAllDrafts,
  purgeDraftsForTab,
  purgeExpiredDrafts,
  readDraft,
  touchedFields,
  writeDraft,
} from './form-drafts'

// What these pin: a draft only ever comes back for the admin and the tab it was typed in, it
// expires, and storage failures degrade to "no persistence" rather than a broken page

afterEach(() => {
  localStorage.clear()
  vi.restoreAllMocks()
})

describe('form drafts', () => {
  it('stores only the fields that moved, with the value they moved from', () => {
    const touched = touchedFields(
      { name: 'a', city: 'x', hours: { mon: 1 }, secret: 's' },
      { name: 'b', city: 'x', hours: { mon: 2 }, secret: 't' },
      ['secret'],
    )
    expect(touched).toEqual({
      name: { base: 'a', value: 'b' },
      hours: { base: { mon: 1 }, value: { mon: 2 } },
    })
  })

  it('keys drafts per admin and per tab', () => {
    writeDraft(draftKey('admin-1', 'tab-a', 'loc:new'), { name: { base: '', value: 'Main' } })
    expect(readDraft(draftKey('admin-1', 'tab-a', 'loc:new'))?.touched.name.value).toBe('Main')
    expect(readDraft(draftKey('admin-2', 'tab-a', 'loc:new'))).toBeNull()
    expect(readDraft(draftKey('admin-1', 'tab-b', 'loc:new'))).toBeNull()
  })

  it('clears instead of writing an empty draft', () => {
    const key = draftKey('admin-1', 'tab-a', 'loc:new')
    writeDraft(key, { name: { base: '', value: 'Main' } })
    writeDraft(key, {})
    expect(localStorage.getItem(key)).toBeNull()
  })

  it('drops drafts past the TTL and ones from another schema version', () => {
    const fresh = draftKey('admin-1', 'tab-a', 'fresh')
    const stale = draftKey('admin-1', 'tab-a', 'stale')
    const foreign = draftKey('admin-1', 'tab-a', 'foreign')
    const now = Date.now()
    writeDraft(fresh, { a: { base: 1, value: 2 } }, now)
    writeDraft(stale, { a: { base: 1, value: 2 } }, now - DRAFT_TTL_MS - 1)
    localStorage.setItem(foreign, JSON.stringify({ v: 99, savedAt: now, touched: {} }))
    purgeExpiredDrafts(now)
    expect(localStorage.getItem(fresh)).not.toBeNull()
    expect(localStorage.getItem(stale)).toBeNull()
    expect(localStorage.getItem(foreign)).toBeNull()
  })

  it('purges one tab, then everything on logout, and leaves other keys alone', () => {
    localStorage.setItem('skelet-dash:auth', 'keep')
    writeDraft(draftKey('admin-1', 'tab-a', 'loc:1'), { a: { base: 1, value: 2 } })
    writeDraft(draftKey('admin-1', 'tab-b', 'loc:1'), { a: { base: 1, value: 2 } })
    purgeDraftsForTab('tab-a')
    expect(readDraft(draftKey('admin-1', 'tab-a', 'loc:1'))).toBeNull()
    expect(readDraft(draftKey('admin-1', 'tab-b', 'loc:1'))).not.toBeNull()
    purgeAllDrafts()
    expect(readDraft(draftKey('admin-1', 'tab-b', 'loc:1'))).toBeNull()
    expect(localStorage.getItem('skelet-dash:auth')).toBe('keep')
  })

  it('swallows storage failures', () => {
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new DOMException('full', 'QuotaExceededError')
    })
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new DOMException('blocked', 'SecurityError')
    })
    const key = draftKey('admin-1', 'tab-a', 'loc:new')
    expect(() => writeDraft(key, { a: { base: 1, value: 2 } })).not.toThrow()
    expect(readDraft(key)).toBeNull()
    expect(() => clearDraft(key)).not.toThrow()
  })
})
