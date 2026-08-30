import { useEffect, type ReactNode } from 'react'
import { useQuery } from '@apollo/client/react'
import { useAuth } from '@/shared/auth/auth'
import { MyPreferencesDocument } from '@/lib/AdminPrefs.gql'
import {
  ANY_KEY,
  LOADED_KEY,
  PrefsContext,
  flushKey,
  getStore,
  notify,
  storageKey,
} from './admin-prefs'

// Split from admin-prefs so that module exports only hooks - fast refresh needs the separation, and
// the 20-odd hook importers keep their path

export function AdminPrefsProvider({ children }: { children: ReactNode }) {
  const { auth } = useAuth()
  const adminID = auth?.admin.id ?? ''
  const query = useQuery(MyPreferencesDocument, { skip: !adminID })

  useEffect(() => {
    if (!query.data || !adminID) return
    const store = getStore(adminID)
    for (const pref of query.data.myPreferences) {
      // a key the user already touched this session is newer than the server copy
      if (store.dirty.has(pref.key)) continue
      store.values.set(pref.key, pref.value)
      try {
        localStorage.setItem(storageKey(adminID, pref.key), JSON.stringify(pref.value))
      } catch {
        /* mirror only */
      }
      notify(store, pref.key)
    }
    store.version += 1
    store.loaded = true
    notify(store, ANY_KEY)
    notify(store, LOADED_KEY)
  }, [query.data, adminID])

  useEffect(() => {
    if (!adminID) return
    // pagehide covers reloads, tab close and cross-site navigation; visibilitychange covers
    // tab switches - both flush whatever is still debouncing
    const flushAll = (unloading: boolean) => {
      for (const key of [...getStore(adminID).timers.keys()]) flushKey(adminID, key, unloading)
    }
    const onHide = () => {
      if (document.visibilityState === 'hidden') flushAll(false)
    }
    const onPageHide = () => flushAll(true)
    document.addEventListener('visibilitychange', onHide)
    window.addEventListener('pagehide', onPageHide)
    return () => {
      document.removeEventListener('visibilitychange', onHide)
      window.removeEventListener('pagehide', onPageHide)
    }
  }, [adminID])

  return <PrefsContext.Provider value={adminID}>{children}</PrefsContext.Provider>
}
