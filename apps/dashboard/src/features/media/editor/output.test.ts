import { describe, expect, it, vi } from 'vitest'
import { NEUTRAL } from './adjust'
import { editedName, isEditable, renderEdited } from './output'

function canvas(pixels: number[]) {
  const data = new Uint8ClampedArray(pixels)
  const put = vi.fn()
  const toBlob = vi.fn((done: (b: Blob | null) => void, type: string) => done(new Blob(['x'], { type })))
  const el = { width: 1, height: 1, getContext: () => ({ getImageData: () => ({ data }), putImageData: put }), toBlob } as unknown as HTMLCanvasElement
  return { el, data, put, toBlob }
}

describe('the edited file', () => {
  it('is named after the original, once', () => {
    expect(editedName('boot.jpg')).toBe('boot-edited.jpg')
    expect(editedName('boot-edited.jpg')).toBe('boot-edited.jpg')
    expect(editedName('Summer sale.final.PNG')).toBe('Summer sale.final-edited.PNG')
    expect(editedName('noext')).toBe('noext-edited')
  })

  it('opens only finished pictures a canvas can write back', () => {
    expect(isEditable({ mimeType: 'image/jpeg', status: 'READY' })).toBe(true)
    expect(isEditable({ mimeType: 'image/webp', status: 'PENDING' })).toBe(false)
    for (const mimeType of ['image/svg+xml', 'image/gif', 'image/heic', 'application/pdf', 'video/mp4']) expect(isEditable({ mimeType, status: 'READY' })).toBe(false)
  })

  it('keeps the type of the original, with a quality only where the type has one', async () => {
    for (const [mime, quality] of [['image/jpeg', 0.92], ['image/webp', 0.92], ['image/png', undefined]] as const) {
      const c = canvas([10, 20, 30, 255])
      const blob = await renderEdited(c.el, NEUTRAL, mime)
      expect(blob.type).toBe(mime)
      expect(c.toBlob).toHaveBeenCalledWith(expect.any(Function), mime, quality)
      expect(c.put).not.toHaveBeenCalled()
    }
  })

  it('works light and colour into the pixels before writing them', async () => {
    const c = canvas([100, 100, 100, 255])
    await renderEdited(c.el, { ...NEUTRAL, brightness: 50 }, 'image/png')
    expect([...c.data]).toEqual([150, 150, 150, 255])
    expect(c.put).toHaveBeenCalledBefore(c.toBlob)
  })

  it('says so when the browser hands back nothing', async () => {
    const c = canvas([0, 0, 0, 255])
    c.toBlob.mockImplementation((done: (b: Blob | null) => void) => done(null))
    await expect(renderEdited(c.el, NEUTRAL, 'image/png')).rejects.toThrow("The edited picture couldn't be written.")
  })
})
