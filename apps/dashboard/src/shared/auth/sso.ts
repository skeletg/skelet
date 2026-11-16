// What the starting tab keeps while the provider has the browser - the binding never leaves it, so a sign-in someone
// else started can't be finished here
const KEY = 'skelet-dash:sso'

// A sign-in, or the signed-in admin connecting their provider account - both come back to the same page
export type PendingSso = { state: string; binding: string; redirectTo: string; mode?: 'signin' | 'link' }

export function ssoReturnUrl(): string {
  return `${window.location.origin}${import.meta.env.BASE_URL}login/sso`
}

export function keepPendingSso(p: PendingSso) {
  try {
    sessionStorage.setItem(KEY, JSON.stringify(p))
  } catch {
    // a browser refusing storage simply can't complete, and the return page says so
  }
}

export function takePendingSso(): PendingSso | null {
  try {
    const raw = sessionStorage.getItem(KEY)
    sessionStorage.removeItem(KEY)
    if (!raw) return null
    const p = JSON.parse(raw) as PendingSso
    const inApp = typeof p.redirectTo === 'string' && p.redirectTo.startsWith('/') && !p.redirectTo.startsWith('//')
    return { ...p, redirectTo: inApp ? p.redirectTo : '/' }
  } catch {
    return null
  }
}

const LOCAL = new Set(['localhost', '127.0.0.1', '[::1]'])

// The provider's sign-in address, or null when it's anything a browser shouldn't be sent to - a javascript: one would
// run in the dashboard
export function providerUrl(raw: string): string | null {
  try {
    const u = new URL(raw)
    return u.protocol === 'https:' || (u.protocol === 'http:' && LOCAL.has(u.hostname)) ? u.href : null
  } catch {
    return null
  }
}

// Keeps what the starting tab needs and sends it to the provider - false when the address isn't one to follow
export function goToProvider(start: { authorizeUrl: string; state: string; binding: string }, redirectTo: string, mode: PendingSso['mode']): boolean {
  const target = providerUrl(start.authorizeUrl)
  if (!target) return false
  keepPendingSso({ state: start.state, binding: start.binding, redirectTo, mode })
  window.location.assign(target)
  return true
}
