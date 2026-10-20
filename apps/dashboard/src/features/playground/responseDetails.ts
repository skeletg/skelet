export type ResponseDetails = {
  status: number
  statusText: string
  durationMs: number
  sizeBytes: number
  errorCount: number
  dbQueries: number | null // dev debug headers, absent outside APP_ENV=development
  traceId: string | null
  requestId: string | null
}

export type ResponseTone = 'success' | 'warning' | 'destructive'

// GraphiQL fetches the schema through the same fetcher on every mount and surface switch - counting it would overwrite the last real response
export function isIntrospectionRequest(body: BodyInit | null | undefined): boolean {
  if (typeof body !== 'string') return false
  try {
    const parsed = JSON.parse(body) as { operationName?: unknown; query?: unknown }
    if (parsed.operationName === 'IntrospectionQuery') return true
    return typeof parsed.query === 'string' && /^\s*query\s+IntrospectionQuery\b/.test(parsed.query)
  } catch {
    return false
  }
}

export function describeResponse(response: Response, bodyText: string, durationMs: number): ResponseDetails {
  let errors: { extensions?: Record<string, unknown> }[] = []
  try {
    const parsed = JSON.parse(bodyText) as { errors?: unknown }
    if (Array.isArray(parsed.errors)) errors = parsed.errors
  } catch {
    // a proxy error page or an empty body - the status carries the story
  }
  const requestId = errors.find((e) => typeof e.extensions?.request_id === 'string')?.extensions?.request_id
  const dbQueries = Number(response.headers.get('x-db-queries') ?? NaN)
  return {
    status: response.status,
    statusText: response.statusText,
    durationMs: Math.round(durationMs),
    sizeBytes: new TextEncoder().encode(bodyText).length,
    errorCount: errors.length,
    dbQueries: Number.isFinite(dbQueries) ? dbQueries : null,
    traceId: response.headers.get('x-trace-id') || null,
    requestId: (requestId as string | undefined) ?? response.headers.get('x-request-id') ?? null,
  }
}

// GraphQL answers errors with a 200, so the status alone would paint a failed query green
export function responseTone(d: ResponseDetails): ResponseTone {
  if (d.status < 200 || d.status >= 300) return 'destructive'
  return d.errorCount > 0 ? 'warning' : 'success'
}

export function formatDuration(ms: number): string {
  return ms < 1000 ? `${ms} ms` : `${(ms / 1000).toFixed(2)} s`
}

// A clone is read alongside GraphiQL's own read, so the time runs to the last body byte rather than the headers
export async function timedFetch(
  input: RequestInfo | URL,
  init: RequestInit | undefined,
  onDetails: (details: ResponseDetails) => void,
): Promise<Response> {
  const started = performance.now()
  const response = await fetch(input, init)
  if (!isIntrospectionRequest(init?.body)) {
    void response
      .clone()
      .text()
      .then((text) => onDetails(describeResponse(response, text, performance.now() - started)))
      .catch(() => {})
  }
  return response
}
