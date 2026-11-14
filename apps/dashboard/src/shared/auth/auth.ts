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
  announce(next)
}

let signedIn = readStored()?.admin?.id ?? null
type AdminChange = (before: string | null, now: string | null) => void
const adminListeners = new Set<AdminChange>()

function announce(next: AuthState | null) {
  for (const l of listeners) l(next)
  const now = next?.admin?.id ?? null
  if (now === signedIn) return
  const before = signedIn
  signedIn = now
  for (const l of adminListeners) l(before, now)
}

// Another tab signing in or out writes the same storage, and this tab has to hear it as if it were its own
if (typeof window !== 'undefined') {
  window.addEventListener('storage', (e) => e.key === STORAGE_KEY && announce(readStored()))
}

// When the admin behind this tab changes - signed out, or someone else in, here or in another tab - and not on a renewal
export function onAdminChange(fn: AdminChange): () => void {
  adminListeners.add(fn)
  return () => {
    adminListeners.delete(fn)
  }
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
