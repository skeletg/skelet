import { act, renderHook } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { useMediaUpload } from '../useImageUpload'
import { useUploadQueue } from './useUploadQueue'

// What this pins: each file gets its own row and progress, a refused file never uploads and keeps its reason, a failed
// one stays with the error while the rest finish, only the finished ids reach onUploaded, and finished rows clear on
// the next batch or on demand

vi.mock('../useImageUpload', () => ({ useMediaUpload: vi.fn() }))

type Pending = { name: string; progress: (f: number) => void; resolve: (id: string) => void; reject: (e: Error) => void }

function rig() {
  const pending: Pending[] = []
  const upload = vi.fn(
    (file: File, progress: (f: number) => void) =>
      new Promise<{ id: string; url: string }>((res, rej) => {
        pending.push({ name: file.name, progress, resolve: (id) => res({ id, url: `https://cdn/${id}` }), reject: rej })
      }),
  )
  vi.mocked(useMediaUpload).mockReturnValue(upload as unknown as ReturnType<typeof useMediaUpload>)
  const onUploaded = vi.fn()
  const hook = renderHook(() => useUploadQueue('option-pictures', onUploaded))
  return { pending, upload, onUploaded, hook }
}

const png = (name: string) => new File(['x'], name, { type: 'image/png' })
const onlyPictures = (f: File) => (f.type.startsWith('image/') ? null : 'Not an image')

afterEach(() => vi.clearAllMocks())

describe('useUploadQueue', () => {
  it('uploads into its folder and tracks each file on its own', async () => {
    const { pending, onUploaded, hook } = rig()
    expect(useMediaUpload).toHaveBeenCalledWith('option-pictures')
    let done: Promise<void> = Promise.resolve()
    act(() => {
      done = hook.result.current.add([png('a.png'), png('b.png')], onlyPictures)
    })
    expect(hook.result.current.busy).toBe(true)
    expect(hook.result.current.items.map((i) => [i.name, i.status])).toEqual([['a.png', 'uploading'], ['b.png', 'uploading']])

    act(() => pending[1].progress(0.4))
    expect(hook.result.current.items.map((i) => i.progress)).toEqual([0, 0.4])

    await act(async () => {
      pending[0].resolve('m-a')
      pending[1].resolve('m-b')
      await done
    })
    expect(hook.result.current.items.map((i) => [i.status, i.progress, i.assetId])).toEqual([['done', 1, 'm-a'], ['done', 1, 'm-b']])
    expect(hook.result.current.busy).toBe(false)
    expect(onUploaded).toHaveBeenCalledExactlyOnceWith(['m-a', 'm-b'])
  })

  it('keeps a refused file as an error and never uploads it', async () => {
    const { upload, onUploaded, hook } = rig()
    await act(() => hook.result.current.add([new File(['x'], 'notes.txt', { type: 'text/plain' })], onlyPictures))
    expect(upload).not.toHaveBeenCalled()
    expect(hook.result.current.items).toMatchObject([{ name: 'notes.txt', status: 'error', error: 'Not an image' }])
    expect(onUploaded).not.toHaveBeenCalled()
  })

  it('passes on only what finished when one fails', async () => {
    const { pending, onUploaded, hook } = rig()
    let done: Promise<void> = Promise.resolve()
    act(() => {
      done = hook.result.current.add([png('good.png'), png('bad.png')], onlyPictures)
    })
    await act(async () => {
      pending[0].resolve('m-good')
      pending[1].reject(new Error('Upload failed with HTTP 413.'))
      await done
    })
    expect(hook.result.current.items.map((i) => [i.name, i.status, i.error])).toEqual([
      ['good.png', 'done', undefined],
      ['bad.png', 'error', 'Upload failed with HTTP 413.'],
    ])
    expect(onUploaded).toHaveBeenCalledExactlyOnceWith(['m-good'])
  })

  it('clears finished rows on the next batch, and dismisses or clears on demand', async () => {
    const { pending, hook } = rig()
    let done: Promise<void> = Promise.resolve()
    act(() => {
      done = hook.result.current.add([png('a.png'), new File(['x'], 'b.txt')], onlyPictures)
    })
    await act(async () => {
      pending[0].resolve('m-a')
      await done
    })
    act(() => {
      void hook.result.current.add([png('c.png')], onlyPictures)
    })
    // the finished one went, the refused one stays until it's dismissed
    expect(hook.result.current.items.map((i) => [i.name, i.status])).toEqual([['b.txt', 'error'], ['c.png', 'uploading']])

    act(() => hook.result.current.dismiss(hook.result.current.items[0].key))
    expect(hook.result.current.items.map((i) => i.name)).toEqual(['c.png'])
    act(() => hook.result.current.clearFinished())
    expect(hook.result.current.items.map((i) => i.name)).toEqual(['c.png'])
  })
})
