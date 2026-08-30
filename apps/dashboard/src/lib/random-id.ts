// crypto.randomUUID exists only in a SECURE CONTEXT - https, or localhost. Reaching the dev
// server over the LAN (http://192.168.x.x:5173, e.g. from a phone) is not one, so the bare call
// is undefined there and throws. It did exactly that in the app shell, which unmounted the whole
// tree and left a white screen after login
// Always route through this rather than calling crypto.randomUUID directly
export function randomId(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID()
  }
  // Not cryptographically strong, and it doesn't need to be - every caller is minting a local
  // key for a list row, tab or draft, never a secret or anything the server trusts
  return `id_${Math.random().toString(36).slice(2)}${Date.now().toString(36)}`
}
