import { ADMIN_GRAPHQL_ENDPOINT } from '@/lib/api-endpoint'
import { getAccessToken, subscribeAuth } from './auth'

// The Signals API does not share core's session store, so the dashboard exchanges its session for a short-lived token - a plain fetch, since routing it through the client that needs its result is how you get a loop

// Re-mint this far before expiry, so an in-flight request never carries a token that dies mid-request
const RENEW_MARGIN_MS = 60_000

type Cached = { token: string; expiresAt: number }

let cached: Cached | null = null
let inflight: Promise<string | null> | null = null

// Any change to the admin session invalidates what was minted from it - a logout must not leave a usable analytics token behind
subscribeAuth(() => {
  cached = null
  inflight = null
})

function fresh(c: Cached | null): c is Cached {
  return c !== null && c.expiresAt - RENEW_MARGIN_MS > Date.now()
}

async function mint(): Promise<string | null> {
  const admin = getAccessToken()
  if (!admin) return null
  const res = await fetch(ADMIN_GRAPHQL_ENDPOINT, {
    method: 'POST',
    headers: { 'content-type': 'application/json', authorization: `Bearer ${admin}` },
    body: JSON.stringify({ query: '{ signalsToken { token expiresAt } }' }),
  })
  if (!res.ok) return null
  const body = (await res.json()) as {
    data?: { signalsToken?: { token: string; expiresAt: string } }
  }
  const minted = body.data?.signalsToken
  if (!minted) return null
  cached = { token: minted.token, expiresAt: Date.parse(minted.expiresAt) }
  return cached.token
}

export async function getSignalsToken(): Promise<string | null> {
  if (fresh(cached)) return cached.token
  // One mint at a time - a page opening three Prism panels at once should not ask for three tokens
  inflight ??= mint().finally(() => {
    inflight = null
  })
  return inflight
}

export function clearSignalsToken() {
  cached = null
  inflight = null
}
