import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react'
import { AuthContext, getAuthStoredState, setAuthStoredState, subscribeAuth, type AuthState } from './auth'

// Split from auth.ts so that module carries only the store and the hook - fast refresh needs a file
// to export either components or non-components, and the hook has far more importers than this

export function AuthProvider({ children }: { children: ReactNode }) {
  const [auth, setAuthLocal] = useState<AuthState | null>(() => getAuthStoredState())

  // Follows the module-level store, which hears non-React code (the refresh link) and other tabs too
  useEffect(() => subscribeAuth(setAuthLocal), [])

  const setAuth = useCallback((next: AuthState | null) => {
    setAuthStoredState(next)
  }, [])

  const value = useMemo(() => ({ auth, setAuth }), [auth, setAuth])
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}
