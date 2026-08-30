import { createContext, useContext } from 'react'

export const STORAGE_KEY = 'skelet-dash:auth'

export type AuthRole = {
  id: string
  slug: string
  name: string
}

export type AuthAdmin = {
  id: string
  email: string
  firstName: string
  lastName: string
  isSuperuser: boolean
  roles: AuthRole[]
}

export type AuthState = {
  accessToken: string
  refreshToken: string
  admin: AuthAdmin
}

// ---------- Module-level auth store ----------
// localStorage is the source of truth - React state (see AuthProvider) is
// just a mirror that subscribes to changes - This lets non-React code -
// namely the Apollo error link that performs silent token refresh - update
// auth without going through a React callback

type Listener = (state: AuthState | null) => void
const listeners = new Set<Listener>()

export function readStored(): AuthState | null {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    return raw ? (JSON.parse(raw) as AuthState) : null
  } catch {
    return null
  }
}

export function getAuthStoredState(): AuthState | null {
  return readStored()
}

export function setAuthStoredState(next: AuthState | null) {
  if (next) localStorage.setItem(STORAGE_KEY, JSON.stringify(next))
  else localStorage.removeItem(STORAGE_KEY)
  for (const l of listeners) l(next)
}

export function subscribeAuth(fn: Listener): () => void {
  listeners.add(fn)
  return () => {
    listeners.delete(fn)
  }
}

export function getAccessToken(): string | null {
  return readStored()?.accessToken ?? null
}

// ---------- React bindings ----------

type AuthContextValue = {
  auth: AuthState | null
  setAuth: (next: AuthState | null) => void
}

export const AuthContext = createContext<AuthContextValue | null>(null)

export function useAuth() {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuth must be used within <AuthProvider>')
  return ctx
}
