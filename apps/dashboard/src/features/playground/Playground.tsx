import { useMemo, useState } from 'react'
import { createGraphiQLFetcher } from '@graphiql/toolkit'
import { GraphiQL } from 'graphiql'
import 'graphiql/setup-workers/vite' // without it MonacoEnvironment.getWorker is unset, the monaco-graphql worker never loads, and the editor silently loses autocomplete and validation
import 'graphiql/style.css'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { getAccessToken } from '@/shared/auth/auth'
import { usePermissions } from '@/shared/auth/usePermissions'
import { exampleTabs, frontExampleTabs } from './examples'
import { OperationsPluginContent, OperationsPluginIcon } from './OperationsPlugin'
import { ResponseDetailsBar } from './ResponseDetailsBar'
import { timedFetch, type ResponseDetails } from './responseDetails'
import { namespacedStorage, SURFACES, surfaceById, type SurfaceId } from './surfaces'

// GraphiQL's own docs explorer browses types, which is a different question from "what can I call"
const operationsPlugin = {
  title: 'Operations',
  icon: OperationsPluginIcon,
  content: OperationsPluginContent,
}

// The admin's token rides every request the IDE issues, so introspection and live queries honour the same guards the dashboard does - auth.Middleware runs on both endpoints and injects admin claims either way
export function Playground() {
  const { has, loading, isSuperuser } = usePermissions()
  const [surfaceId, setSurfaceId] = useState<SurfaceId>('admin')
  const surface = surfaceById(surfaceId)
  // Tagged with its surface so a response still in flight when the admin switches can't land on the other tab
  const [last, setLast] = useState<{ surfaceId: SurfaceId; details: ResponseDetails } | null>(null)

  const fetcher = useMemo(
    () =>
      createGraphiQLFetcher({
        url: surface.endpoint,
        fetch: (input, init) => {
          const token = getAccessToken()
          const headers = new Headers(init?.headers)
          if (token) headers.set('Authorization', `Bearer ${token}`)
          headers.set('Content-Type', 'application/json')
          return timedFetch(input, { ...init, headers }, (details) => setLast({ surfaceId: surface.id, details }))
        },
      }),
    [surface.endpoint, surface.id],
  )

  const storage = useMemo(() => namespacedStorage(surfaceId), [surfaceId]) // per surface, so switching back finds the same editor

  if (loading) return <p className="text-sm text-muted-foreground">Loading…</p>

  if (!isSuperuser && !has('developer.playground')) {
    return (
      <Alert variant="destructive">
        <AlertTitle>Access denied</AlertTitle>
        <AlertDescription>
          You need the <code className="font-mono">developer.playground</code> permission to use
          the GraphQL playground.
        </AlertDescription>
      </Alert>
    )
  }

  return (
    <div className="flex h-full min-h-0 flex-col gap-3">
      <div className="flex flex-wrap items-center gap-3">
        <div role="tablist" aria-label="GraphQL surface" className="flex rounded-md border p-0.5">
          {SURFACES.map((s) => (
            <button
              key={s.id}
              role="tab"
              type="button"
              aria-selected={s.id === surfaceId}
              onClick={() => setSurfaceId(s.id)}
              className={
                s.id === surfaceId
                  ? 'rounded-sm bg-primary px-3 py-1 text-sm font-medium text-primary-foreground'
                  : 'rounded-sm px-3 py-1 text-sm text-muted-foreground hover:text-foreground'
              }
            >
              {s.label}
            </button>
          ))}
        </div>
        <p className="text-xs text-muted-foreground">
          {surface.hint} - <code className="font-mono">{surface.endpoint}</code>
        </p>
      </div>
      <div className="min-h-0 flex-1 overflow-hidden rounded-lg border">
        {/* Remount per surface: the schema, tabs and storage all change together */}
        <GraphiQL
          key={surfaceId}
          fetcher={fetcher}
          storage={storage}
          plugins={[operationsPlugin]}
          defaultTabs={surfaceId === 'admin' ? exampleTabs : frontExampleTabs}
        >
          <GraphiQL.Footer>
            <ResponseDetailsBar details={last?.surfaceId === surfaceId ? last.details : null} />
          </GraphiQL.Footer>
        </GraphiQL>
      </div>
    </div>
  )
}
