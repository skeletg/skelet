import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react'
import {
  AuthContext,
  STORAGE_KEY,
  getAuthStoredState,
  readStored,
  setAuthStoredState,
  subscribeAuth,
  type AuthState,
} from './auth'

// Split from auth.ts so that module carries only the store and the hook - fast refresh needs a file
// to export either components or non-components, and the hook has far more importers than this

export function AuthProvider({ children }: { children: ReactNode }) {
  const [auth, setAuthLocal] = useState<AuthState | null>(() => getAuthStoredState())

  // Sync local state whenever the module-level store changes - including
  // changes made by non-React code (refresh link)
  useEffect(() => subscribeAuth(setAuthLocal), [])

  // Cross-tab sync via the `storage` event (not dispatched in the same tab)
  useEffect(() => {
    const onStorage = (e: StorageEvent) => {
      if (e.key === STORAGE_KEY) setAuthLocal(readStored())
    }
    window.addEventListener('storage', onStorage)
    return () => window.removeEventListener('storage', onStorage)
  }, [])

  const setAuth = useCallback((next: AuthState | null) => {
    setAuthStoredState(next)
  }, [])

  const value = useMemo(() => ({ auth, setAuth }), [auth, setAuth])
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}
