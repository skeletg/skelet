import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  describeResponse,
  formatBytes,
  formatDuration,
  isIntrospectionRequest,
  responseTone,
  timedFetch,
} from './responseDetails'

function response(status: number, headers: Record<string, string> = {}) {
  return new Response(null, { status, statusText: status === 200 ? 'OK' : '', headers })
}

describe('isIntrospectionRequest', () => {
  it('spots GraphiQL schema fetches by operation name or query', () => {
    expect(isIntrospectionRequest(JSON.stringify({ operationName: 'IntrospectionQuery', query: '...' }))).toBe(true)
    expect(isIntrospectionRequest(JSON.stringify({ query: '  query IntrospectionQuery { __schema { types { name } } }' }))).toBe(true)
  })

  it('leaves real operations, including ones that mention __schema, alone', () => {
    expect(isIntrospectionRequest(JSON.stringify({ operationName: 'Products', query: 'query Products { products { id } }' }))).toBe(false)
    expect(isIntrospectionRequest(JSON.stringify({ query: '{ __schema { queryType { name } } }' }))).toBe(false)
    expect(isIntrospectionRequest('not json')).toBe(false)
    expect(isIntrospectionRequest(undefined)).toBe(false)
  })
})

describe('describeResponse', () => {
  it('reads the status, byte size, errors and the dev debug headers', () => {
    const body = JSON.stringify({ errors: [{ message: 'no', extensions: { request_id: 'req-1' } }, { message: 'also no' }], data: { név: 'é' } })
    const d = describeResponse(response(200, { 'X-DB-Queries': '7', 'X-Trace-Id': 'abc123' }), body, 141.6)
    expect(d).toMatchObject({ status: 200, durationMs: 142, errorCount: 2, dbQueries: 7, traceId: 'abc123', requestId: 'req-1' })
    expect(d.sizeBytes).toBe(new TextEncoder().encode(body).length)
    expect(d.sizeBytes).toBeGreaterThan(body.length) // bytes, not UTF-16 characters
  })

  it('falls back to the X-Request-Id header and survives a non-JSON body', () => {
    const d = describeResponse(response(502, { 'X-Request-Id': 'hdr-9' }), '<html>Bad Gateway</html>', 12)
    expect(d).toMatchObject({ status: 502, errorCount: 0, dbQueries: null, traceId: null, requestId: 'hdr-9' })
  })
})

describe('responseTone', () => {
  const base = describeResponse(response(200), '{"data":{}}', 1)
  it('paints a 200 with GraphQL errors as a warning, not a success', () => {
    expect(responseTone(base)).toBe('success')
    expect(responseTone({ ...base, errorCount: 1 })).toBe('warning')
    expect(responseTone({ ...base, status: 500 })).toBe('destructive')
  })
})

describe('formatting', () => {
  it('scales bytes and durations', () => {
    expect(formatBytes(512)).toBe('512 B')
    expect(formatBytes(3482)).toBe('3.4 KB')
    expect(formatBytes(5 * 1024 * 1024)).toBe('5.0 MB')
    expect(formatDuration(142)).toBe('142 ms')
    expect(formatDuration(2345)).toBe('2.35 s')
  })
})

describe('timedFetch', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('reports a real operation once its body is read, and hands GraphiQL an unread response', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response('{"data":{"me":null}}', { status: 200, headers: { 'X-DB-Queries': '3' } })))
    const onDetails = vi.fn()
    const res = await timedFetch('/api', { method: 'POST', body: JSON.stringify({ query: '{ me { id } }' }) }, onDetails)
    expect(await res.json()).toEqual({ data: { me: null } })
    await vi.waitFor(() => expect(onDetails).toHaveBeenCalledTimes(1))
    expect(onDetails.mock.calls[0][0]).toMatchObject({ status: 200, dbQueries: 3, sizeBytes: 20 })
  })

  it('stays quiet for the schema fetch', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response('{"data":{"__schema":{}}}')))
    const onDetails = vi.fn()
    await timedFetch('/api', { method: 'POST', body: JSON.stringify({ operationName: 'IntrospectionQuery', query: 'query IntrospectionQuery {}' }) }, onDetails)
    await new Promise((r) => setTimeout(r, 10))
    expect(onDetails).not.toHaveBeenCalled()
  })
})
