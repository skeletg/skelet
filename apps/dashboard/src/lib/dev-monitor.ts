import { useSyncExternalStore } from 'react'
import { ApolloLink } from '@apollo/client'
import { map } from 'rxjs'

// Dev-only request monitor - an Apollo link recording per operation the backend's dev debug headers (X-DB-Queries, X-Trace-Id) and any
// FORBIDDEN denial details the dev error presenter attached - The dev inspector renders the ring buffer, and it's only wired in under DEV

export type DevRequestEntry = {
  id: number
  operation: string
  at: number
  dbQueries: number | null
  traceId: string | null
  requestId: string | null
  errorMessage: string | null
  deniedPermission: string | null
  deniedChannelId: string | null
  deniedLocale: string | null
}

const MAX_ENTRIES = 30
let entries: DevRequestEntry[] = []
let seq = 0
const listeners = new Set<() => void>()

function notify() {
  for (const fn of listeners) fn()
}

function subscribe(fn: () => void): () => void {
  listeners.add(fn)
  return () => listeners.delete(fn)
}

function getSnapshot(): DevRequestEntry[] {
  return entries
}

/** React hook over the ring buffer - newest first */
export function useDevRequests(): DevRequestEntry[] {
  return useSyncExternalStore(subscribe, getSnapshot)
}

type GraphQLErrorShape = {
  message?: string
  extensions?: Record<string, unknown>
}

function record(operationName: string, response: Response | undefined, errors: GraphQLErrorShape[] | undefined) {
  const first = errors?.[0]
  const ext = first?.extensions ?? {}
  const str = (v: unknown) => (typeof v === 'string' && v !== '' ? v : null)
  const entry: DevRequestEntry = {
    id: ++seq,
    operation: operationName || '(anonymous)',
    at: Date.now(),
    dbQueries: response ? Number(response.headers.get('x-db-queries') ?? NaN) || null : null,
    traceId: response?.headers.get('x-trace-id') ?? null,
    requestId: str(ext['request_id']),
    errorMessage: first?.message ?? null,
    deniedPermission: str(ext['denied_permission']),
    deniedChannelId: str(ext['denied_channel_id']),
    deniedLocale: str(ext['denied_locale']),
  }
  // New array identity so useSyncExternalStore consumers re-render
  entries = [entry, ...entries].slice(0, MAX_ENTRIES)
  notify()
}

export const devMonitorLink = new ApolloLink((operation, forward) =>
  forward(operation).pipe(
    map((result) => {
      try {
        const response = operation.getContext().response as Response | undefined
        const errors = (result as { errors?: GraphQLErrorShape[] }).errors
        record(operation.operationName ?? '', response, errors)
      } catch {
        // Monitoring must never break the request path
      }
      return result
    }),
  ),
)
