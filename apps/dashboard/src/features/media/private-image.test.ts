import { renderHook, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ADMIN_MEDIA_FILE_ENDPOINT } from '@/lib/api-endpoint'
import { STORAGE_KEY, setAuthStoredState } from '@/shared/auth/auth'
import { usePrivateImage, usePrivatePicture } from './private-image'


const calls = () => vi.mocked(fetch).mock.calls as unknown as [string, { headers: Record<string, string> }][]
const admin = (id: string) => ({ accessToken: 'tok', refreshToken: 'r', admin: { id, email: '', firstName: '', lastName: '', isSuperuser: false, roles: [] } })

beforeEach(() => {
  localStorage.setItem(STORAGE_KEY, JSON.stringify(admin('a1')))
  let n = 0
  URL.createObjectURL = vi.fn(() => `blob:picture-${++n}`)
  URL.revokeObjectURL = vi.fn()
  vi.stubGlobal('fetch', vi.fn(async () => new Response(new Blob(['png']))))
})
afterEach(() => {
  localStorage.clear()
  vi.unstubAllGlobals()
})

describe('usePrivateImage', () => {
  it('fetches the picture with the session at the width asked for', async () => {
    const { result } = renderHook(() => usePrivateImage('one', 640))
    expect(result.current).toEqual({ url: null, failed: false })
    await waitFor(() => expect(result.current.url).toBe('blob:picture-1'))
    expect(calls()).toEqual([[`${ADMIN_MEDIA_FILE_ENDPOINT}/one?w=640`, { headers: { authorization: 'Bearer tok' } }]])
  })

  it('asks once for a file shown in two places, and again for another width', async () => {
    const a = renderHook(() => usePrivateImage('two', 640))
    const b = renderHook(() => usePrivateImage('two', 640))
    await waitFor(() => expect([a.result.current.url, b.result.current.url]).toEqual(['blob:picture-1', 'blob:picture-1']))
    expect(calls()).toHaveLength(1)

    const whole = renderHook(() => usePrivateImage('two'))
    await waitFor(() => expect(whole.result.current.url).toBe('blob:picture-2'))
    expect(calls().map(([url]) => url)).toEqual([`${ADMIN_MEDIA_FILE_ENDPOINT}/two?w=640`, `${ADMIN_MEDIA_FILE_ENDPOINT}/two`])
  })

  it('asks for nothing when there is no private file to show', () => {
    const { result } = renderHook(() => usePrivateImage(null, 640))
    expect(result.current).toEqual({ url: null, failed: false })
    expect(fetch).not.toHaveBeenCalled()
  })

  it('drops the picture of the file it showed before', async () => {
    const { result, rerender } = renderHook(({ id }) => usePrivateImage(id), { initialProps: { id: 'three' } })
    await waitFor(() => expect(result.current.url).toBe('blob:picture-1'))
    rerender({ id: 'four' })
    expect(result.current.url).toBeNull()
    await waitFor(() => expect(result.current.url).toBe('blob:picture-2'))
  })

  it('says when the server refused, and asks again the next time', async () => {
    vi.mocked(fetch).mockResolvedValueOnce(new Response('no', { status: 403 }))
    const refused = renderHook(() => usePrivateImage('five'))
    await waitFor(() => expect(refused.result.current).toEqual({ url: null, failed: true }))

    const again = renderHook(() => usePrivateImage('five'))
    await waitFor(() => expect(again.result.current.url).toBe('blob:picture-1'))
    expect(calls()).toHaveLength(2)
  })

  it('lets go of every picture when the session ends or another admin signs in, and not on a renewed token', async () => {
    setAuthStoredState(admin('a1'))
    const first = renderHook(() => usePrivateImage('eight', 640))
    await waitFor(() => expect(first.result.current.url).not.toBeNull())
    const url = first.result.current.url!
    vi.mocked(URL.revokeObjectURL).mockClear()

    setAuthStoredState({ ...admin('a1'), accessToken: 'renewed' })
    expect(URL.revokeObjectURL).not.toHaveBeenCalled()
    setAuthStoredState(null)
    await waitFor(() => expect(URL.revokeObjectURL).toHaveBeenCalledWith(url))

    setAuthStoredState(admin('a2'))
    const again = renderHook(() => usePrivateImage('eight', 640))
    await waitFor(() => expect(again.result.current.url).not.toBeNull())
    expect(calls().filter(([u]) => u.endsWith('/eight?w=640'))).toHaveLength(2)
  })

  it('keeps what it holds within its budget, oldest out first, and never one still on its way', async () => {
    setAuthStoredState(null)
    setAuthStoredState(admin('a3'))
    let n = 0
    URL.createObjectURL = vi.fn(() => `blob:budget-${++n}`)
    // raw bytes - a jsdom Blob handed to a Response arrives as the 13 characters of its name
    const big = (mb: number) => new Response(new Uint8Array(mb << 20))
    let arrive: (r: Response) => void = () => {}
    vi.mocked(fetch).mockImplementation(async () => big(40))
    vi.mocked(fetch).mockImplementationOnce(() => new Promise<Response>((r) => (arrive = r)))

    const slow = renderHook(() => usePrivateImage('slow'))
    const pictures = ['big-1', 'big-2', 'big-3'].map((id) => renderHook(() => usePrivateImage(id)))
    await waitFor(() => expect(pictures.every((p) => p.result.current.url)).toBe(true))
    const revoked = () => vi.mocked(URL.revokeObjectURL).mock.calls.map(([u]) => u)
    const held = pictures.map((p) => p.result.current.url)
    await waitFor(() => expect(revoked()).toContain(held[0]))
    expect(held.filter((u) => revoked().includes(u!))).toEqual([held[0]])

    arrive(big(1))
    await waitFor(() => expect(slow.result.current.url).not.toBeNull())
    expect(revoked()).not.toContain(slow.result.current.url)
  })
})

describe('usePrivatePicture', () => {
  const asset = (over: object) => ({ id: 'p', visibility: 'PRIVATE' as const, mimeType: 'image/jpeg', sizeBytes: 2048, ...over })

  it('asks for a copy the server resizes, a small picture whole, and nothing for what it would not draw', async () => {
    const cases: [object, string | null][] = [
      [{ id: 'jpeg' }, `${ADMIN_MEDIA_FILE_ENDPOINT}/jpeg?w=640`],
      [{ id: 'gif', mimeType: 'image/gif' }, `${ADMIN_MEDIA_FILE_ENDPOINT}/gif`],
      [{ id: 'huge-gif', mimeType: 'image/gif', sizeBytes: 40 << 20 }, null],
      [{ id: 'svg', mimeType: 'image/svg+xml' }, null],
      [{ id: 'heic', mimeType: 'image/heic' }, null],
      [{ id: 'public', visibility: 'PUBLIC' }, null],
    ]
    for (const [over, want] of cases) {
      vi.mocked(fetch).mockClear()
      const { result, unmount } = renderHook(() => usePrivatePicture(asset(over), 640))
      expect(result.current.shown).toBe(want !== null)
      if (want) await waitFor(() => expect(calls().map(([u]) => u)).toEqual([want]))
      else expect(fetch).not.toHaveBeenCalled()
      unmount()
    }
  })
})
