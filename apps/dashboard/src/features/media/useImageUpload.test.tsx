import type { ReactNode } from 'react'
import { renderHook } from '@testing-library/react'
import { MockedProvider } from '@apollo/client/testing/react'
import type { MockLink } from '@apollo/client/testing'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ADMIN_MEDIA_SVG_ENDPOINT } from '@/lib/api-endpoint'
import { readImageDimensions } from '@/lib/image'
import { putToSignedUrlWithProgress } from '@/lib/upload'
import { STORAGE_KEY } from '@/shared/auth/auth'
import { ConfirmMediaAssetUploadDocument, RequestMediaAssetUploadDocument } from './MediaLibrary.gql'
import { useImageUpload, useMediaUpload } from './useImageUpload'

// What this pins: one upload goes request -> signed PUT (with progress) -> confirm into the folder it was given, with the
// picture's size stamped when the browser can read it, an SVG goes through the server instead, and the image-only
// wrapper refuses anything else before asking for a slot

vi.mock('@/lib/upload', () => ({ putToSignedUrlWithProgress: vi.fn() }))
vi.mock('@/lib/image', () => ({ readImageDimensions: vi.fn() }))

const put = vi.mocked(putToSignedUrlWithProgress)
const dims = vi.mocked(readImageDimensions)

afterEach(() => {
  vi.unstubAllGlobals()
  vi.clearAllMocks()
  localStorage.clear()
})

const asset = (over: object = {}) => ({
  __typename: 'MediaAsset' as const, id: 'm1', path: 'option-pictures/a.png', directoryPath: 'option-pictures', filename: 'a.png',
  url: 'https://cdn/a.png', mimeType: 'image/png', mimeGroup: 'IMAGE' as const, sizeBytes: 3, width: null, height: null, status: 'PENDING' as const,
  title: '', altText: '', caption: '', notes: '', focalX: null, focalY: null, createdAt: '2026-10-01T00:00:00Z', updatedAt: '2026-10-01T00:00:00Z',
  ...over,
})

function rig({ confirmed = asset({ status: 'READY', width: 40, height: 30 }) }: { confirmed?: ReturnType<typeof asset> | null } = {}) {
  const requested: Record<string, unknown>[] = []
  const confirmedWith: Record<string, unknown>[] = []
  const mocks: MockLink.MockedResponse[] = [
    {
      request: {
        query: RequestMediaAssetUploadDocument,
        variables: (v: { input: Record<string, unknown> }) => {
          requested.push(v.input)
          return true
        },
      },
      result: { data: { requestMediaAssetUpload: { __typename: 'RequestMediaAssetUploadPayload', uploadUrl: 'https://bucket/signed', expiresAt: '2026-10-01T01:00:00Z', asset: asset() } } },
    },
    {
      request: {
        query: ConfirmMediaAssetUploadDocument,
        variables: (v: Record<string, unknown>) => {
          confirmedWith.push(v)
          return true
        },
      },
      result: { data: { confirmMediaAssetUpload: confirmed } },
    },
  ]
  const wrapper = ({ children }: { children: ReactNode }) => <MockedProvider mocks={mocks}>{children}</MockedProvider>
  return { requested, confirmedWith, wrapper }
}

describe('useMediaUpload', () => {
  it('asks for a slot in its folder, PUTs with progress and confirms with the size', async () => {
    const { requested, confirmedWith, wrapper } = rig()
    dims.mockResolvedValue({ width: 40, height: 30 })
    put.mockImplementation(async (_url, _file, onProgress) => {
      onProgress(0.5)
      onProgress(1)
    })
    const { result } = renderHook(() => useMediaUpload('option-pictures'), { wrapper })
    const file = new File(['abc'], 'a.png', { type: 'image/png' })
    const progress: number[] = []
    const got = await result.current(file, (f) => progress.push(f))

    expect(requested).toEqual([{ path: 'option-pictures', filename: 'a.png', mimeType: 'image/png', sizeBytes: 3, altText: '' }])
    expect(put).toHaveBeenCalledWith('https://bucket/signed', file, expect.any(Function))
    expect(progress).toEqual([0.5, 1])
    expect(confirmedWith).toEqual([{ assetId: 'm1', width: 40, height: 30 }])
    expect(got).toMatchObject({ id: 'm1', status: 'READY', width: 40 })
  })

  it('still uploads a picture the browser cannot decode, just without its size', async () => {
    const { confirmedWith, wrapper } = rig()
    dims.mockRejectedValue(new Error('Could not read image dimensions.'))
    const { result } = renderHook(() => useMediaUpload('uploads'), { wrapper })
    await result.current(new File(['abc'], 'a.heic', { type: 'image/heic' }))
    expect(confirmedWith).toEqual([{ assetId: 'm1' }])
  })

  it('sends a file without a type as octet-stream and never measures it', async () => {
    const { requested, wrapper } = rig()
    const { result } = renderHook(() => useMediaUpload('uploads'), { wrapper })
    await result.current(new File(['abc'], 'manual.bin'))
    expect(requested[0]).toMatchObject({ mimeType: 'application/octet-stream' })
    expect(dims).not.toHaveBeenCalled()
  })

  it('throws when the confirm comes back empty', async () => {
    const { wrapper } = rig({ confirmed: null })
    dims.mockResolvedValue({ width: 1, height: 1 })
    const { result } = renderHook(() => useMediaUpload('uploads'), { wrapper })
    await expect(result.current(new File(['abc'], 'a.png', { type: 'image/png' }))).rejects.toThrow('Upload confirmation failed.')
  })

  it('posts an SVG through the server, signed in, so it gets sanitized', async () => {
    localStorage.setItem(STORAGE_KEY, JSON.stringify({ accessToken: 'tok', refreshToken: 'r', admin: {} }))
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, status: 200, json: async () => ({ id: 's1', url: 'https://cdn/logo.svg' }) })
    vi.stubGlobal('fetch', fetchMock)
    const { requested, wrapper } = rig()
    const { result } = renderHook(() => useMediaUpload('brand'), { wrapper })
    const progress: number[] = []
    const got = await result.current(new File(['<svg/>'], 'logo.svg', { type: 'image/svg+xml' }), (f) => progress.push(f))

    expect(got).toEqual({ id: 's1', url: 'https://cdn/logo.svg' })
    expect(progress).toEqual([1])
    expect(requested).toEqual([])
    const [url, init] = fetchMock.mock.calls[0] as [string, { method: string; headers: Record<string, string>; body: FormData }]
    expect([url, init.method, init.headers.Authorization]).toEqual([ADMIN_MEDIA_SVG_ENDPOINT, 'POST', 'Bearer tok'])
    expect(init.body.get('path')).toBe('brand')
  })

  it("passes on the server's reason when it refuses an SVG", async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: false, status: 422, json: async () => ({ error: 'SVG has a script in it' }) }))
    const { wrapper } = rig()
    const { result } = renderHook(() => useMediaUpload('brand'), { wrapper })
    await expect(result.current(new File(['<svg/>'], 'x.svg', { type: 'image/svg+xml' }))).rejects.toThrow('SVG has a script in it')
  })
})

describe('useImageUpload', () => {
  it('hands back the id and url of the uploaded picture', async () => {
    const { wrapper } = rig()
    dims.mockResolvedValue({ width: 40, height: 30 })
    const { result } = renderHook(() => useImageUpload('option-pictures'), { wrapper })
    expect(await result.current(new File(['abc'], 'a.png', { type: 'image/png' }))).toEqual({ assetId: 'm1', url: 'https://cdn/a.png' })
  })

  it('refuses a file that is not a picture before asking for a slot', async () => {
    const { requested, wrapper } = rig()
    const { result } = renderHook(() => useImageUpload('option-pictures'), { wrapper })
    await expect(result.current(new File(['x'], 'notes.txt', { type: 'text/plain' }))).rejects.toThrow('Only image files can be uploaded.')
    expect(requested).toEqual([])
  })
})
