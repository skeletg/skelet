import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { readImageDimensions } from './image'

// jsdom never loads images, so a stub Image fires onload or onerror as soon as src is set
let outcome: 'load' | 'error' = 'load'

class FakeImage {
  naturalWidth = 640
  naturalHeight = 480
  onload: (() => void) | null = null
  onerror: (() => void) | null = null
  set src(_: string) {
    queueMicrotask(() => (outcome === 'load' ? this.onload?.() : this.onerror?.()))
  }
}

const revoke = vi.fn()

beforeEach(() => {
  outcome = 'load'
  revoke.mockClear()
  vi.stubGlobal('Image', FakeImage)
  vi.stubGlobal('URL', { ...URL, createObjectURL: () => 'blob:fake', revokeObjectURL: revoke })
})

afterEach(() => {
  vi.unstubAllGlobals()
})

const file = () => new File(['x'], 'photo.png', { type: 'image/png' })

describe('readImageDimensions', () => {
  it('resolves the natural size and releases the object URL', async () => {
    await expect(readImageDimensions(file())).resolves.toEqual({ width: 640, height: 480 })
    expect(revoke).toHaveBeenCalledWith('blob:fake')
  })

  it('rejects an unreadable image and still releases the object URL', async () => {
    outcome = 'error'
    await expect(readImageDimensions(file())).rejects.toThrow('Could not read image dimensions.')
    expect(revoke).toHaveBeenCalledWith('blob:fake')
  })
})
