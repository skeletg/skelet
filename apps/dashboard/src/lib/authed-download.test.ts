import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ADMIN_DOWNLOAD_TICKET_ENDPOINT, ADMIN_GRAPHQL_ENDPOINT } from '@/lib/api-endpoint'
import { STORAGE_KEY, readStored } from '@/shared/auth/auth'
import { downloadServer } from '@/test/downloads'
import { downloadWithSession, fetchWithSession } from './authed-download'


const admin = { id: 'a1', email: 'ada@example.com', firstName: 'Ada', lastName: 'Admin', isSuperuser: false, roles: [] }
const asked = () => (vi.mocked(fetch).mock.calls as unknown as [string, { headers?: Record<string, string>; body?: string }][]).map(([url, init]) => [url, init.headers?.authorization ?? JSON.parse(init.body ?? '{}').variables?.token])

function server(answers: Record<string, Response[]>) {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string) => answers[url]!.shift()!),
  )
}

beforeEach(() => localStorage.setItem(STORAGE_KEY, JSON.stringify({ accessToken: 'old', refreshToken: 'r1', admin })))
afterEach(() => {
  localStorage.clear()
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

describe('fetchWithSession', () => {
  it('renews a session that has run out and asks again with the new one', async () => {
    const renewed = { accessToken: 'new', refreshToken: 'r2', admin }
    server({
      '/api/admin/media/file/x': [new Response('', { status: 401 }), new Response('picture')],
      [ADMIN_GRAPHQL_ENDPOINT]: [Response.json({ data: { refreshToken: renewed } })],
    })
    const res = await fetchWithSession('/api/admin/media/file/x')
    expect(await res.text()).toBe('picture')
    expect(asked()).toEqual([
      ['/api/admin/media/file/x', 'Bearer old'],
      [ADMIN_GRAPHQL_ENDPOINT, 'r1'],
      ['/api/admin/media/file/x', 'Bearer new'],
    ])
    expect(readStored()?.accessToken).toBe('new')
  })

  it('signs out when the session cannot be renewed, and hands back the refusal', async () => {
    server({
      '/api/admin/media/file/x': [new Response('', { status: 401 })],
      [ADMIN_GRAPHQL_ENDPOINT]: [Response.json({ errors: [{ message: 'refresh token revoked' }] })],
    })
    expect((await fetchWithSession('/api/admin/media/file/x')).status).toBe(401)
    expect(readStored()).toBeNull()
  })

  it('leaves a refusal for a missing grant alone - renewing the session would not change it', async () => {
    server({ '/api/admin/media/file/x': [new Response('forbidden', { status: 403 })] })
    expect((await fetchWithSession('/api/admin/media/file/x')).status).toBe(403)
    expect(asked()).toEqual([['/api/admin/media/file/x', 'Bearer old']])
    expect(readStored()?.accessToken).toBe('old')
  })

  it('renews once for a wave of requests that all found the session run out', async () => {
    const renewed = { accessToken: 'new', refreshToken: 'r2', admin }
    let renewals = 0
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string, init: { headers?: Record<string, string> }) => {
        if (url === ADMIN_GRAPHQL_ENDPOINT) {
          renewals++
          await new Promise((r) => setTimeout(r, 10))
          return Response.json({ data: { refreshToken: renewed } })
        }
        return init.headers?.authorization === 'Bearer new' ? new Response('picture') : new Response('', { status: 401 })
      }),
    )
    const answers = await Promise.all(['a', 'b', 'c'].map((id) => fetchWithSession(`/api/admin/media/file/${id}`)))
    expect(answers.map((r) => r.status)).toEqual([200, 200, 200])
    expect(renewals).toBe(1)
  })
})

describe('downloadWithSession', () => {
  it('buys a link for the one file with the session, tries it, and lets the browser save it - nothing held in memory', async () => {
    URL.createObjectURL = vi.fn(() => 'blob:never')
    const { asked, saved } = downloadServer(() => new Response('csv', { headers: { 'content-disposition': 'attachment; filename="customers.csv"' } }))
    await downloadWithSession('/api/admin/data/template?resource=customers&format=csv', 'fallback.csv')
    expect(saved).toEqual([{ href: '/api/admin/data/template?resource=customers&format=csv&ticket=t1', name: 'customers.csv' }])
    expect(asked.map((a) => [a.method, a.url.replace(window.location.origin, ''), a.authorization])).toEqual([
      ['POST', ADMIN_DOWNLOAD_TICKET_ENDPOINT, 'Bearer old'],
      ['GET', '/api/admin/data/template?resource=customers&format=csv&ticket=t1', undefined],
    ])
    expect(URL.createObjectURL).not.toHaveBeenCalled()
  })

  it("says why, and saves nothing, when the link or the ticket is refused", async () => {
    const files = downloadServer(() => new Response('this export has expired', { status: 410 }))
    await expect(downloadWithSession('/api/admin/data/jobs/j1/download', 'x.csv')).rejects.toThrow('this export has expired')
    expect(files.saved).toEqual([])

    vi.stubGlobal('fetch', vi.fn(async () => new Response("that isn't a file the dashboard downloads", { status: 400 })))
    await expect(downloadWithSession('/api/admin/graphql', 'x')).rejects.toThrow("that isn't a file the dashboard downloads")
    expect(files.saved).toEqual([])
  })
})
