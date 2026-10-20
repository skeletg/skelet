import { afterEach, describe, expect, it, vi } from 'vitest'
import { putToSignedUrl } from './upload'

// What this pins: the shared signed-URL PUT sends the file with its own content type, and a refused upload throws with
// the caller's label and the status, so each flow still says what failed
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
