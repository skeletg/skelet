import { afterEach, describe, expect, it, vi } from 'vitest'
import { STORAGE_KEY, onAdminChange, setAuthStoredState } from './auth'


const as = (id: string, token = 'tok') => ({ accessToken: token, refreshToken: 'r', admin: { id, email: '', firstName: '', lastName: '', isSuperuser: false, roles: [] } })

afterEach(() => localStorage.clear())

describe('onAdminChange', () => {
  it('tells of a sign-out and of another admin, here or in another tab, but not of a renewed token', () => {
    setAuthStoredState(as('a1'))
    const seen = vi.fn()
    const stop = onAdminChange(seen)

    setAuthStoredState(as('a1', 'renewed'))
    setAuthStoredState(null)
    setAuthStoredState(as('a2'))
    localStorage.setItem(STORAGE_KEY, JSON.stringify(as('a3')))
    window.dispatchEvent(new StorageEvent('storage', { key: STORAGE_KEY }))
    window.dispatchEvent(new StorageEvent('storage', { key: 'something-else' }))
    expect(seen.mock.calls).toEqual([
      ['a1', null],
      [null, 'a2'],
      ['a2', 'a3'],
    ])
    stop()
    setAuthStoredState(null)
    expect(seen).toHaveBeenCalledTimes(3)
  })
})
