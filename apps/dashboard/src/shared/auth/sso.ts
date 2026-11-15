// What the starting tab keeps while the provider has the browser - the binding never leaves it, so a sign-in someone
// else started can't be finished here
const KEY = 'skelet-dash:sso'

export type PendingSso = { state: string; binding: string; redirectTo: string }

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
    return raw ? (JSON.parse(raw) as PendingSso) : null
  } catch {
    return null
  }
}
