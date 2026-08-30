import { createContext, useCallback, useContext, useSyncExternalStore } from 'react'
import { apolloClient } from '@/lib/apollo'
import { getAuthStoredState } from '@/shared/auth/auth'
import { SetMyPreferenceDocument } from '@/lib/AdminPrefs.gql'
import { ADMIN_GRAPHQL_ENDPOINT } from '@/lib/api-endpoint'

// Per-admin UI preferences backed by the admin_preferences table so they roam across devices -
// table layouts, saved list views, nav pins, home layout. Reads are per-key subscriptions
// (useSyncExternalStore) so a chatty writer like a column-resize drag re-renders only its own
// consumers; writes are optimistic with a localStorage mirror and a per-key debounce to the
// setMyPreference mutation. The store lives at module scope: React state must not be mutated,
// and pending debounce timers have to survive a page unmount to still flush

const DEBOUNCE_MS = 750
export const LOADED_KEY = '__loaded' // internal sentinel channel for usePrefsLoaded subscribers
export const ANY_KEY = '__any' // store-wide channel - a key LIST can't subscribe per key

type Store = {
  values: Map<string, unknown>
  subscribers: Map<string, Set<() => void>>
  dirty: Set<string> // keys written locally since mount - the server hydrate must not clobber them
  timers: Map<string, ReturnType<typeof setTimeout>>
  loaded: boolean
  version: number // bumped on every key add/remove so key lists know to recompute
  keyCache: Map<string, { version: number; keys: string[] }> // useSyncExternalStore loops on a fresh array every call
}

const stores = new Map<string, Store>()

export function storageKey(adminID: string, key: string) {
  return `prefs:${adminID}:${key}`
}

// One store per admin, hydrated from the device mirror on first touch so layouts don't flash defaults while the roaming
// copy loads - keyed per admin so accounts on a shared browser don't bleed
export function getStore(adminID: string): Store {
  let store = stores.get(adminID)
  if (store) return store
  store = {
    values: new Map(),
    subscribers: new Map(),
    dirty: new Set(),
    timers: new Map(),
    loaded: false,
    version: 0,
    keyCache: new Map(),
  }
  if (adminID) {
    try {
      const prefix = storageKey(adminID, '')
      for (let i = 0; i < localStorage.length; i++) {
        const k = localStorage.key(i)
        if (!k?.startsWith(prefix)) continue
        const raw = localStorage.getItem(k)
        if (raw) store.values.set(k.slice(prefix.length), JSON.parse(raw))
      }
    } catch {
      /* corrupt mirror or private mode - the server copy still arrives */
    }
  }
  stores.set(adminID, store)
  return store
}

export function notify(store: Store, key: string) {
  for (const cb of store.subscribers.get(key) ?? []) cb()
}

function subscribeKey(adminID: string, key: string, cb: () => void): () => void {
  const store = getStore(adminID)
  let set = store.subscribers.get(key)
  if (!set) {
    set = new Set()
    store.subscribers.set(key, set)
  }
  set.add(cb)
  return () => set.delete(cb)
}

// Sorted keys under a prefix, cached so the snapshot stays referentially stable - the identity is
// held across unrelated key changes too, or every column-drag write would re-render every key list
function keysWithPrefix(adminID: string, prefix: string): string[] {
  const store = getStore(adminID)
  const hit = store.keyCache.get(prefix)
  if (hit && hit.version === store.version) return hit.keys
  const keys = [...store.values.keys()].filter((k) => k.startsWith(prefix)).sort()
  const same =
    hit && hit.keys.length === keys.length && hit.keys.every((k, i) => k === keys[i])
  const cached = same ? hit.keys : keys
  store.keyCache.set(prefix, { version: store.version, keys: cached })
  return cached
}

export function flushKey(adminID: string, key: string, unloading = false) {
  const store = getStore(adminID)
  const t = store.timers.get(key)
  if (t === undefined) return
  clearTimeout(t)
  store.timers.delete(key)
  const value = store.values.get(key)
  const variables = { key, value: value === undefined ? null : value }
  if (unloading) {
    // the document is going away, so Apollo's fetch would be aborted mid-flight - a raw
    // keepalive request survives the unload (and 64KB keepalive budget >> the 32KB value cap)
    const token = getAuthStoredState()?.accessToken
    if (!token) return
    void fetch(ADMIN_GRAPHQL_ENDPOINT, {
      method: 'POST',
      keepalive: true,
      headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
      body: JSON.stringify({
        query: 'mutation($key: String!, $value: JSON) { setMyPreference(key: $key, value: $value) }',
        variables,
      }),
    }).catch(() => {
      /* the mirror still holds it - next session reconciles */
    })
    return
  }
  void apolloClient
    .mutate({ mutation: SetMyPreferenceDocument, variables })
    .catch(() => {
      /* the mirror still holds it - next session reconciles */
    })
}

function storeSet(adminID: string, key: string, next: unknown) {
  const store = getStore(adminID)
  store.dirty.add(key)
  const had = store.values.has(key)
  if (next === undefined) store.values.delete(key)
  else store.values.set(key, next)
  if (had !== store.values.has(key)) store.version += 1
  try {
    if (next === undefined) localStorage.removeItem(storageKey(adminID, key))
    else localStorage.setItem(storageKey(adminID, key), JSON.stringify(next))
  } catch {
    /* mirror only */
  }
  notify(store, key)
  notify(store, ANY_KEY)
  const existing = store.timers.get(key)
  if (existing !== undefined) clearTimeout(existing)
  store.timers.set(
    key,
    setTimeout(() => flushKey(adminID, key), DEBOUNCE_MS),
  )
}

export const PrefsContext = createContext<string | null>(null) // the acting admin's id

function useAdminID(): string {
  const id = useContext(PrefsContext)
  if (id === null) throw new Error('usePreference must be used within AdminPrefsProvider')
  return id
}

// One roaming preference - undefined-in-store falls back to the given default, and the setter
// accepts a value or an updater like useState; passing undefined deletes the key
export function usePreference<T>(
  key: string,
  fallback: T,
): [T, (next: T | undefined | ((prev: T) => T | undefined)) => void] {
  const adminID = useAdminID()
  const stored = useSyncExternalStore(
    useCallback((cb) => subscribeKey(adminID, key, cb), [adminID, key]),
    () => getStore(adminID).values.get(key),
  )
  const value = stored === undefined ? fallback : (stored as T)
  const set = useCallback(
    (next: T | undefined | ((prev: T) => T | undefined)) => {
      const prev = getStore(adminID).values.get(key)
      const resolved =
        typeof next === 'function'
          ? (next as (prev: T) => T | undefined)((prev === undefined ? fallback : prev) as T)
          : next
      storeSet(adminID, key, resolved)
    },
    [adminID, key, fallback],
  )
  return [value, set]
}

// Read-only, and tolerant of no provider at all - for shared chrome like DetailPageLayout and the
// table toolbar, which render in page tests and must not hard-require the store to say "default"
export function useOptionalPreference<T>(key: string, fallback: T): T {
  const adminID = useContext(PrefsContext) ?? '' // the anonymous store exists and stays empty
  const stored = useSyncExternalStore(
    useCallback((cb) => subscribeKey(adminID, key, cb), [adminID, key]),
    () => getStore(adminID).values.get(key),
  )
  return stored === undefined ? fallback : (stored as T)
}

// True once the roaming copy has arrived - gates behavior that must not act on defaults, like
// saved-view auto-apply
export function usePrefsLoaded(): boolean {
  const adminID = useAdminID()
  return useSyncExternalStore(
    useCallback((cb) => subscribeKey(adminID, LOADED_KEY, cb), [adminID]),
    () => getStore(adminID).loaded,
  )
}

// Every stored key under a prefix - the Preferences page enumerates `views:` and `table:` to offer
// resets, which per-key access can't express
export function usePreferenceKeys(prefix: string): string[] {
  const adminID = useAdminID()
  return useSyncExternalStore(
    useCallback((cb) => subscribeKey(adminID, ANY_KEY, cb), [adminID]),
    () => keysWithPrefix(adminID, prefix),
  )
}

// Bulk delete - one debounced mutation per key, which is fine for the dozens a reset touches
export function useRemovePreferences(): (keys: string[]) => void {
  const adminID = useAdminID()
  return useCallback(
    (keys: string[]) => {
      for (const key of keys) storeSet(adminID, key, undefined)
    },
    [adminID],
  )
}
