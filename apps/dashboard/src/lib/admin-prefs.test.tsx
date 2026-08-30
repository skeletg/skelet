import { act, cleanup, render } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { usePreference, usePreferenceKeys, useRemovePreferences } from './admin-prefs'
import { AdminPrefsProvider } from './admin-prefs-provider'

// The provider needs an admin identity, an Apollo query for the roaming copy and a mutate for
// write-through - all faked so the store's own bookkeeping is what's under test
const { state } = vi.hoisted(() => ({
  state: { adminID: '', queryData: undefined as unknown, mutations: [] as unknown[] },
}))

vi.mock('@/shared/auth/auth', () => ({
  useAuth: () => ({ auth: { admin: { id: state.adminID }, accessToken: 't' } }),
  getAuthStoredState: () => ({ accessToken: 't' }),
}))
vi.mock('@apollo/client/react', () => ({
  useQuery: () => ({ data: state.queryData }),
}))
vi.mock('@/lib/apollo', () => ({
  apolloClient: {
    mutate: (opts: unknown) => {
      state.mutations.push(opts)
      return Promise.resolve({})
    },
  },
}))

let seq = 0
afterEach(cleanup)
beforeEach(() => {
  // a fresh admin id per test - the store is module-scoped and deliberately survives unmounts
  seq += 1
  state.adminID = `admin-${seq}`
  state.queryData = undefined
  state.mutations = []
  localStorage.clear()
})

function mountKeys(prefix: string) {
  const renders: string[][] = []
  let write!: (key: string, value: unknown) => void
  let remove!: (keys: string[]) => void
  function Probe() {
    const keys = usePreferenceKeys(prefix)
    const [, setA] = usePreference<number>('table:a', 0)
    const [, setViews] = usePreference<number>('views:x', 0)
    const removeKeys = useRemovePreferences()
    renders.push(keys)
    write = (key, value) => (key === 'views:x' ? setViews(value as number) : setA(value as number))
    remove = removeKeys
    return null
  }
  render(
    <AdminPrefsProvider>
      <Probe />
    </AdminPrefsProvider>,
  )
  return {
    renders,
    get latest() {
      return renders[renders.length - 1]
    },
    write: (key: string, value: unknown) => act(() => write(key, value)),
    remove: (keys: string[]) => act(() => remove(keys)),
  }
}

describe('usePreferenceKeys', () => {
  it('returns only matching keys, sorted', () => {
    const h = mountKeys('table:')
    h.write('table:a', 1)
    h.write('views:x', 1)
    expect(h.latest).toEqual(['table:a'])
  })

  it('keeps the snapshot referentially stable when an unrelated key changes', () => {
    const h = mountKeys('table:')
    h.write('table:a', 1)
    const before = h.latest
    h.write('views:x', 2) // different prefix, so the table: list must not be rebuilt
    expect(h.latest).toBe(before)
  })

  it('re-renders the list when a matching key appears', () => {
    const h = mountKeys('table:')
    expect(h.latest).toEqual([])
    h.write('table:a', 1)
    expect(h.latest).toEqual(['table:a'])
  })

  it('publishes keys that only exist on the server', () => {
    state.queryData = {
      myPreferences: [{ key: 'table:server', value: { d: true }, updatedAt: '2026-01-01' }],
    }
    const h = mountKeys('table:')
    expect(h.latest).toEqual(['table:server'])
  })
})

describe('useRemovePreferences', () => {
  it('clears the store entry, the device mirror and queues the delete', () => {
    const h = mountKeys('table:')
    h.write('table:a', 7)
    expect(localStorage.getItem(`prefs:${state.adminID}:table:a`)).toBe('7')

    h.remove(['table:a'])
    expect(h.latest).toEqual([])
    expect(localStorage.getItem(`prefs:${state.adminID}:table:a`)).toBeNull()
  })
})
