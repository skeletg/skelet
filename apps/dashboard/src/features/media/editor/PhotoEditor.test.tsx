import { forwardRef, useEffect, useImperativeHandle } from 'react'
import { act, fireEvent, screen, waitFor, within } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { STORAGE_KEY } from '@/shared/auth/auth'
import { SLOW, captureInput, renderDialog } from '@/test/page-harness'
import { UpdateMediaAssetDocument } from '../MediaLibrary.gql'
import { useMediaUpload } from '../useImageUpload'
import PhotoEditor from './PhotoEditor'

type Box = { width: number; height: number }
const stage = vi.hoisted(() => ({
  props: null as Record<string, unknown> | null,
  api: null as Record<string, (...args: never[]) => unknown> | null,
  turned: 0,
  box: { width: 1200, height: 800 } as Box,
  calls: [] as string[],
  pixels: new Uint8ClampedArray([100, 100, 100, 255]),
  written: null as { type: string } | null,
}))

vi.mock('react-advanced-cropper/dist/style.css', () => ({}))
vi.mock('react-advanced-cropper', () => ({
  Cropper: forwardRef(function FakeCropper(props: Record<string, unknown>, ref) {
    const state = () => ({ imageSize: { width: 1200, height: 800 }, transforms: { rotate: stage.turned } })
    const api = {
      rotateImage: (by: number) => void ((stage.turned += by), stage.calls.push(`rotate ${by}`)),
      flipImage: (across: boolean, down: boolean) => void stage.calls.push(`flip ${across} ${down}`),
      reset: () => void ((stage.turned = 0), (stage.box = { width: 1200, height: 800 }), stage.calls.push('reset')),
      // as the library does: the largest box of the stencil's shape inside what was asked for, then word of the change
      setCoordinates: (to: (s: ReturnType<typeof state>) => Box) => {
        const full = to(state())
        const shape = (stage.props!.stencilProps as { aspectRatio?: number }).aspectRatio
        const width = shape ? Math.min(full.width, full.height * shape) : full.width
        stage.box = { width: Math.round(width), height: Math.round(shape ? width / shape : full.height) }
        stage.calls.push('whole')
        ;(stage.props!.onUpdate as (c: unknown) => void)(api)
      },
      getCoordinates: () => stage.box,
      getState: state,
      getCanvas: () => ({
        width: 1,
        height: 1,
        getContext: () => ({ getImageData: () => ({ data: stage.pixels }), putImageData: () => {} }),
        toBlob: (done: (b: Blob) => void, type: string) => ((stage.written = { type }), done(new Blob(['edited'], { type }))),
      }),
    }
    stage.props = props
    stage.api = api as never
    useImperativeHandle(ref, () => api)
    useEffect(() => {
      ;(props.onReady as (c: unknown) => void)?.(api)
    }, []) // eslint-disable-line react-hooks/exhaustive-deps
    const stencil = props.stencilProps as { aspectRatio?: number }
    return <div data-testid="stage" data-src={props.src as string} data-aspect={stencil.aspectRatio ?? 'free'} className={props.backgroundClassName as string} />
  }),
}))
vi.mock('../useImageUpload', () => ({ useMediaUpload: vi.fn() }))

const boot = {
  __typename: 'MediaAsset' as const, id: 'boot', path: 'shoes/boot.jpg', directoryPath: 'shoes', filename: 'boot.jpg', url: 'https://cdn/boot.jpg', thumbnailUrl: 'https://cdn/boot.jpg',
  mimeType: 'image/jpeg', mimeGroup: 'IMAGE' as const, sizeBytes: 2048, width: 1200, height: 800, status: 'READY' as const,
  title: 'Chelsea boot', altText: 'A brown leather boot', caption: '', notes: 'shot in May', focalX: null, focalY: null, createdAt: '2026-10-01T00:00:00Z', updatedAt: '2026-10-01T00:00:00Z',
}

const drag = (box: Box) =>
  act(() => {
    stage.box = box
    ;(stage.props!.onUpdate as (c: unknown) => void)(stage.api)
  })

function open({ upload = vi.fn(async () => ({ id: 'copy-1', url: 'https://cdn/copy.jpg' })), fetched = () => Promise.resolve(new Response(new Blob(['img'], { type: 'image/jpeg' }))) } = {}) {
  const uploadsTo: string[] = []
  vi.mocked(useMediaUpload).mockImplementation((path: string) => (uploadsTo.push(path), upload as unknown as ReturnType<typeof useMediaUpload>))
  const fetching = vi.fn(fetched)
  vi.stubGlobal('fetch', fetching)
  const words: Record<string, unknown>[] = []
  const onClose = vi.fn()
  const onSaved = vi.fn()
  renderDialog(<PhotoEditor asset={boot} onClose={onClose} onSaved={onSaved} />, [
    { request: { query: UpdateMediaAssetDocument, variables: captureInput((i) => words.push(i)) }, result: { data: { updateMediaAsset: { ...boot, id: 'copy-1' } } } },
  ])
  return { upload, uploadsTo, fetching, words, onClose, onSaved }
}
const editor = () => screen.getByRole('dialog')
const size = () => within(editor()).findByLabelText('Size of the edited picture', {}, SLOW)
const save = () => within(editor()).getByRole('button', { name: 'Save as a copy' }) as HTMLButtonElement

beforeEach(() => {
  Object.assign(stage, { props: null, api: null, turned: 0, box: { width: 1200, height: 800 }, calls: [], pixels: new Uint8ClampedArray([100, 100, 100, 255]), written: null })
  localStorage.setItem(STORAGE_KEY, JSON.stringify({ accessToken: 'tok', refreshToken: 'r', admin: {} }))
  URL.createObjectURL = vi.fn(() => 'blob:original')
  URL.revokeObjectURL = vi.fn()
})
afterEach(() => {
  localStorage.clear()
  vi.unstubAllGlobals()
  vi.clearAllMocks()
})

describe('PhotoEditor', () => {
  it('opens the original through the API with the session, whole and unchanged', async () => {
    const { fetching } = open()
    expect((await size()).textContent).toBe('1200 × 800 px')
    const [url, init] = fetching.mock.calls[0] as unknown as [string, { headers: Record<string, string> }]
    expect([url, init.headers.Authorization]).toEqual(['/api/admin/media/file/boot', 'Bearer tok'])
    expect(screen.getByTestId('stage').dataset.src).toBe('blob:original')
    expect(within(editor()).getByText('Edit boot.jpg')).toBeDefined()
    expect(save().disabled).toBe(true)
  })

  it('turns a whole picture whole, mirrors it, and crops to the shape asked for', async () => {
    open()
    await size()
    fireEvent.click(within(editor()).getByRole('button', { name: 'Turn right' }))
    expect(stage.calls).toEqual(['rotate 90', 'whole'])
    expect((await size()).textContent).toBe('800 × 1200 px')

    fireEvent.click(within(editor()).getByRole('button', { name: 'Mirror' }))
    fireEvent.click(within(editor()).getByRole('button', { name: 'Upside down' }))
    expect(stage.calls.slice(2)).toEqual(['flip true false', 'flip false true'])
    stage.calls.length = 0
    expect(within(editor()).getByRole('button', { name: 'Mirror' }).getAttribute('aria-pressed')).toBe('true')

    const aspect = () => Number(screen.getByTestId('stage').dataset.aspect)
    fireEvent.click(within(editor()).getByRole('button', { name: 'Original' }))
    expect(aspect()).toBeCloseTo(800 / 1200)
    fireEvent.click(within(editor()).getByRole('button', { name: '16:9' }))
    expect(aspect()).toBeCloseTo(16 / 9)
    fireEvent.click(within(editor()).getByLabelText('Upright, taller than wide'))
    expect(aspect()).toBeCloseTo(9 / 16)
    fireEvent.click(within(editor()).getByRole('button', { name: 'Square' }))
    expect(aspect()).toBe(1)
    expect(within(editor()).queryByLabelText('Upright, taller than wide')).toBeNull()
    expect(stage.calls).toEqual(['whole', 'whole', 'whole', 'whole'])

    await drag({ width: 300, height: 300 })
    fireEvent.click(within(editor()).getByRole('button', { name: 'Free' }))
    fireEvent.click(within(editor()).getByRole('button', { name: 'Turn left' }))
    expect(stage.calls.slice(4)).toEqual(['rotate -90'])
    expect((await size()).textContent).toBe('300 × 300 px')
  })

  it('frames the largest piece of a chosen shape, which alone is an edit worth saving', async () => {
    open()
    await size()
    fireEvent.click(within(editor()).getByRole('button', { name: 'Square' }))
    await waitFor(async () => expect((await size()).textContent).toBe('800 × 800 px'))
    expect(save().disabled).toBe(false)

    fireEvent.click(within(editor()).getByRole('button', { name: 'Original' }))
    await waitFor(async () => expect((await size()).textContent).toBe('1200 × 800 px'))
    expect(save().disabled).toBe(true)
  })

  it('shows light and colour on the stage and saves them into a copy beside the original', async () => {
    const { upload, uploadsTo, words, onSaved } = open()
    await size()
    const stageBox = screen.getByTestId('stage').parentElement!
    expect(stageBox.style.getPropertyValue('--photo-filter')).toBe('none')
    fireEvent.change(within(editor()).getByLabelText('Brightness'), { target: { value: '50' } })
    expect(stageBox.style.getPropertyValue('--photo-filter')).toBe('brightness(1.5) contrast(1) saturate(1)')
    expect(screen.getByTestId('stage').className).toContain('[filter:var(--photo-filter)]')
    expect(within(editor()).getByText('boot-edited.jpg')).toBeDefined()

    fireEvent.click(save())
    await waitFor(() => expect(onSaved).toHaveBeenCalledWith('copy-1'), SLOW)
    const file = (upload.mock.calls[0] as unknown as [File])[0]
    expect([file.name, file.type, uploadsTo.at(-1)]).toEqual(['boot-edited.jpg', 'image/jpeg', 'shoes'])
    expect(stage.written).toEqual({ type: 'image/jpeg' })
    expect([...stage.pixels]).toEqual([150, 150, 150, 255])
    expect(words).toEqual([{ id: 'copy-1', title: 'Chelsea boot', altText: 'A brown leather boot', caption: '' }])
  })

  it('asks before leaving an edit behind, and not when there is none', async () => {
    const { onClose } = open()
    await size()
    fireEvent.click(within(editor()).getByRole('button', { name: 'Close the editor' }))
    expect(onClose).toHaveBeenCalledTimes(1)

    await drag({ width: 600, height: 800 })
    fireEvent.click(within(editor()).getByRole('button', { name: 'Close the editor' }))
    const ask = await screen.findByRole('alertdialog')
    expect(within(ask).getByText('Leave without saving the edit?')).toBeDefined()
    fireEvent.click(within(ask).getByRole('button', { name: 'Keep editing' }))
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
    expect(onClose).toHaveBeenCalledTimes(1)

    fireEvent.click(within(editor()).getByRole('button', { name: 'Start over' }))
    expect(stage.calls.at(-1)).toBe('reset')
    await drag(stage.box)
    expect(save().disabled).toBe(true)

    await drag({ width: 600, height: 800 })
    fireEvent.click(within(editor()).getByRole('button', { name: 'Close the editor' }))
    fireEvent.click(within(await screen.findByRole('alertdialog')).getByRole('button', { name: 'Leave' }))
    await waitFor(() => expect(onClose).toHaveBeenCalledTimes(2))
  })

  it('says why when the original cannot be opened, and offers nothing to save', async () => {
    open({ fetched: () => Promise.resolve(new Response('no', { status: 415 })) })
    expect((await within(editor()).findByRole('alert', {}, SLOW)).textContent).toBe("This kind of file can't be edited.")
    expect(screen.queryByTestId('stage')).toBeNull()
    expect(save().disabled).toBe(true)
  })

  it('stays open and says why when the copy cannot be saved', async () => {
    const { onSaved } = open({ upload: vi.fn(async () => Promise.reject(new Error('file exceeds max upload size'))) })
    await size()
    await drag({ width: 600, height: 800 })
    fireEvent.click(save())
    expect((await within(editor()).findByRole('alert', {}, SLOW)).textContent).toBe('file exceeds max upload size')
    expect(onSaved).not.toHaveBeenCalled()
    expect(save().disabled).toBe(false)
  })
})
