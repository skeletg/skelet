import { afterEach, describe, expect, it, vi } from 'vitest'
import { putToSignedUrl, putToSignedUrlWithProgress } from './upload'

// What this pins: the shared signed-URL PUT sends the file with its own content type, and a refused upload throws with
// the caller's label and the status, so each flow still says what failed - the progress flavour too, reporting 0..1 on the way
afterEach(() => vi.unstubAllGlobals())

describe('putToSignedUrl', () => {
  it('PUTs the file with its content type', async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, status: 200 })
    vi.stubGlobal('fetch', fetchMock)
    const file = new File(['x'], 'a.png', { type: 'image/png' })
    await putToSignedUrl('https://bucket/signed', file)
    expect(fetchMock).toHaveBeenCalledWith('https://bucket/signed', { method: 'PUT', headers: { 'Content-Type': 'image/png' }, body: file })
  })

  it('throws with the label and status when storage refuses it', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: false, status: 403 }))
    await expect(putToSignedUrl('https://bucket/signed', new File(['x'], 'a.png'), 'Banner upload')).rejects.toThrow('Banner upload failed with HTTP 403.')
  })
})

// Stands in for the browser's XHR - each test drives the upload's progress and its end by hand
class FakeXHR {
  static last: FakeXHR
  method = ''
  url = ''
  headers: Record<string, string> = {}
  body: unknown
  status = 0
  upload: { onprogress: ((e: { lengthComputable: boolean; loaded: number; total: number }) => void) | null } = { onprogress: null }
  onload: (() => void) | null = null
  onerror: (() => void) | null = null
  constructor() {
    FakeXHR.last = this
  }
  open(method: string, url: string) {
    this.method = method
    this.url = url
  }
  setRequestHeader(k: string, v: string) {
    this.headers[k] = v
  }
  send(body: unknown) {
    this.body = body
  }
  finish(status: number) {
    this.status = status
    this.onload?.()
  }
}

describe('putToSignedUrlWithProgress', () => {
  it('PUTs the file and reports how much has gone up', async () => {
    vi.stubGlobal('XMLHttpRequest', FakeXHR)
    const file = new File(['xyz'], 'a.png', { type: 'image/png' })
    const seen: number[] = []
    const done = putToSignedUrlWithProgress('https://bucket/signed', file, (f) => seen.push(f))
    const xhr = FakeXHR.last
    expect([xhr.method, xhr.url, xhr.headers['Content-Type'], xhr.body]).toEqual(['PUT', 'https://bucket/signed', 'image/png', file])
    xhr.upload.onprogress?.({ lengthComputable: true, loaded: 1, total: 4 })
    xhr.upload.onprogress?.({ lengthComputable: false, loaded: 0, total: 0 })
    xhr.upload.onprogress?.({ lengthComputable: true, loaded: 4, total: 4 })
    xhr.finish(200)
    await done
    expect(seen).toEqual([0.25, 1])
  })

  it('rejects with the label and status when storage refuses it', async () => {
    vi.stubGlobal('XMLHttpRequest', FakeXHR)
    const done = putToSignedUrlWithProgress('https://bucket/signed', new File(['x'], 'a.png'), () => {}, 'Picture upload')
    FakeXHR.last.finish(403)
    await expect(done).rejects.toThrow('Picture upload failed with HTTP 403.')
  })

  it('rejects when the connection drops', async () => {
    vi.stubGlobal('XMLHttpRequest', FakeXHR)
    const done = putToSignedUrlWithProgress('https://bucket/signed', new File(['x'], 'a.png'), () => {})
    FakeXHR.last.onerror?.()
    await expect(done).rejects.toThrow('Upload failed - the connection dropped.')
  })
})
