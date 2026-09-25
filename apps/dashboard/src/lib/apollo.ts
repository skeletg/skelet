import { Observable } from 'rxjs'
import { getMainDefinition } from '@apollo/client/utilities'
import { ApolloClient, ApolloLink, InMemoryCache } from '@apollo/client'
import { CombinedGraphQLErrors } from '@apollo/client/errors'
import { SetContextLink } from '@apollo/client/link/context'
import { ErrorLink } from '@apollo/client/link/error'
import { HttpLink } from '@apollo/client/link/http'
import { GraphQLWsLink } from '@apollo/client/link/subscriptions'
import { createClient as createWsClient } from 'graphql-ws'
import {
  getAccessToken,
  getAuthStoredState,
  setAuthStoredState,
  type AuthState,
} from '@/shared/auth/auth'
import { getEffectiveLocale } from '@/lib/locale-store'
import { getEffectiveChannelId } from '@/lib/channel-store'
import { devMonitorLink } from '@/lib/dev-monitor'
import { ADMIN_GRAPHQL_ENDPOINT } from '@/lib/api-endpoint'
import { typePolicies } from '@/lib/apollo-type-policies'
import { setSignalsDown } from '@/shared/signals/health'

const ENDPOINT = ADMIN_GRAPHQL_ENDPOINT

const httpLink = new HttpLink({ uri: ENDPOINT })

const authLink = new SetContextLink((prevContext) => {
  const token = getAccessToken()
  return {
    headers: {
      ...(prevContext.headers ?? {}),
      ...(token ? { authorization: `Bearer ${token}` } : {}),
    },
  }
})

// Reads a module-scoped store the config-scope provider keeps in sync - empty sends no
// header, and the server treats that as "fall back to the system default"
const localeLink = new SetContextLink((prevContext) => {
  const locale = getEffectiveLocale()
  return {
    headers: {
      ...(prevContext.headers ?? {}),
      ...(locale ? { Locale: locale } : {}),
    },
  }
})

// Mirrors localeLink for the `X-Channel-Id` header - empty (Global) sends no header, which the backend's channel.AdminMiddleware reads as
// "all channels", never the default store Channel-targeting operations still pass an explicit channelId argument
const channelLink = new SetContextLink((prevContext) => {
  const channelId = getEffectiveChannelId()
  return {
    headers: {
      ...(prevContext.headers ?? {}),
      ...(channelId ? { 'X-Channel-Id': channelId } : {}),
    },
  }
})

// ---------- Silent refresh ---------- Access tokens live 15 min - on an unauthenticated error, exchange the refresh
// token and retry - Concurrent 401s share one refresh promise, so the server sees one call per wave

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
    const res = await fetch(ENDPOINT, {
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

function getOrStartRefresh(): Promise<AuthState | null> {
  if (!refreshPromise) {
    refreshPromise = performRefresh().finally(() => {
      refreshPromise = null
    })
  }
  return refreshPromise
}

function isUnauthError(error: unknown): boolean {
  if (!error) return false
  if (CombinedGraphQLErrors.is(error)) {
    return error.errors.some((e) =>
      /unauth|not.?authenticated|invalid.?token|expired|jwt/i.test(e.message),
    )
  }
  const maybeHttp = error as { statusCode?: number; status?: number }
  return maybeHttp.statusCode === 401 || maybeHttp.status === 401
}

// Core relays an unreachable suite under one code, so the banner no longer has to guess from a transport failure
function isSignalsUnavailable(error: unknown): boolean {
  return (
    error instanceof CombinedGraphQLErrors &&
    error.errors.some((e) => e.extensions?.code === 'SIGNALS_UNAVAILABLE')
  )
}

const errorLink = new ErrorLink(({ error, operation, forward }) => {
  const opName = operation.operationName
  if (isSignalsUnavailable(error)) setSignalsDown(true)
  if (opName === 'AdminLogin' || opName === 'RefreshToken') return
  if (!isUnauthError(error)) return

  return new Observable<import('@apollo/client').ApolloLink.Result>((observer) => {
    getOrStartRefresh().then((next) => {
      if (!next) {
        setAuthStoredState(null)
        observer.error(error)
        return
      }
      forward(operation).subscribe({
        next: (value) => observer.next(value),
        error: (err) => observer.error(err),
        complete: () => observer.complete(),
      })
    })
  })
})

// Sits directly above the HTTP link so it sees the final response (debug headers, error
// extensions) - the import is static but vite tree-shakes it out of production builds
const terminalLink = import.meta.env.DEV ? devMonitorLink.concat(httpLink) : httpLink

// ---------- Subscriptions (graphql-ws) ----------
// A socket upgrade can't carry an Authorization header, so the JWT goes in the
// connection_init payload; connectionParams is re-read on every reconnect, picking up a
// token refreshed since - Latency layer only - queries stay on HTTP and the bell still polls
const wsUrl = (() => {
  if (/^https?:/i.test(ENDPOINT)) return ENDPOINT.replace(/^http/i, 'ws')
  const proto = window.location.protocol === 'https:' ? 'wss' : 'ws'
  return `${proto}://${window.location.host}${ENDPOINT}`
})()

const wsLink = new GraphQLWsLink(
  createWsClient({
    url: wsUrl,
    lazy: true, // connect on first subscription, not app boot (login page has none)
    connectionParams: () => {
      const token = getAccessToken()
      return token ? { Authorization: `Bearer ${token}` } : {}
    },
    shouldRetry: () => true,
    // Dev-only: a silently failing socket is indistinguishable from "no notifications"
    on: import.meta.env.DEV
      ? {
          connected: () => console.debug('[ws] connected', wsUrl),
          error: (e) => console.warn('[ws] error', e),
          closed: (e) => console.debug('[ws] closed', e),
        }
      : undefined,
  }),
)

// Clears the banner as soon as anything answers without that code - optimistic, and the next read
// that does reach for Signals raises it again
const signalsHealthLink = new ApolloLink(
  (operation, forward) =>
    new Observable<import('@apollo/client').ApolloLink.Result>((observer) => {
      const sub = forward(operation).subscribe({
        next: (result) => {
          const errs = 'errors' in result ? (result.errors ?? []) : []
          if (!errs.some((e) => e.extensions?.code === 'SIGNALS_UNAVAILABLE')) setSignalsDown(false)
          observer.next(result)
        },
        error: (err) => observer.error(err),
        complete: () => observer.complete(),
      })
      return () => sub.unsubscribe()
    }),
)

const transportLink = ApolloLink.split(
  ({ query }) => {
    const def = getMainDefinition(query)
    return def.kind === 'OperationDefinition' && def.operation === 'subscription'
  },
  wsLink,
  signalsHealthLink.concat(terminalLink),
)

export const apolloClient = new ApolloClient({
  link: errorLink.concat(authLink).concat(localeLink).concat(channelLink).concat(transportLink),
  cache: new InMemoryCache({ typePolicies }),
  // Admin panel - both useQuery and client.query() skip cached reads and always hit the
  // network, while the cache still fills so refetchQueries and post-mutation writes work
  defaultOptions: {
    watchQuery: { fetchPolicy: 'network-only', nextFetchPolicy: 'network-only' },
    query: { fetchPolicy: 'network-only' },
  },
})
