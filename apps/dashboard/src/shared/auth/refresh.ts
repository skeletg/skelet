import { ADMIN_GRAPHQL_ENDPOINT } from '@/lib/api-endpoint'
import { getAuthStoredState, setAuthStoredState, type AuthState } from './auth'

// Access tokens live 15 min. A wave of 401s, through Apollo or a plain fetch, shares one refresh

const REFRESH_MUTATION = /* GraphQL */ `
  mutation RefreshToken($token: String!) {
    refreshToken(token: $token) {
      accessToken
      refreshToken
      admin {
        id
        email
        firstName
        lastName
        isSuperuser
        roles {
          id
          slug
          name
        }
      }
    }
  }
`

let refreshPromise: Promise<AuthState | null> | null = null

async function performRefresh(): Promise<AuthState | null> {
  const auth = getAuthStoredState()
  if (!auth?.refreshToken) return null
  try {
    const res = await fetch(ADMIN_GRAPHQL_ENDPOINT, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        query: REFRESH_MUTATION,
        variables: { token: auth.refreshToken },
      }),
    })
    const json = await res.json()
    if (!res.ok || json.errors || !json.data?.refreshToken) return null
    const p = json.data.refreshToken as AuthState
    setAuthStoredState(p)
    return p
  } catch {
    return null
  }
}

export function refreshSession(): Promise<AuthState | null> {
  if (!refreshPromise) {
    refreshPromise = performRefresh().finally(() => {
      refreshPromise = null
    })
  }
  return refreshPromise
}
