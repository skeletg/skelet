import { act, fireEvent, screen, within } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import { SLOW, meMock, renderDialog } from '@/test/page-harness'
import { useMediaUpload } from '../useImageUpload'
import { AllMediaDirectoriesDocument, MediaAssetByIdDocument, SearchMediaAssetsDocument } from '../MediaLibrary.gql'
import { MediaPickerDialog } from './MediaPickerDialog'

// What these pin: a file goes into the open folder or else the caller's, shows its progress, holds Use until it lands,
// and is picked once it does - added to the picks when several are taken - while one that isn't ready yet is not.
// Dropping and pasting take the same road, and the list and the folders refresh after

vi.mock('../useImageUpload', () => ({ useMediaUpload: vi.fn() }))

Element.prototype.hasPointerCapture ??= () => false
Element.prototype.releasePointerCapture ??= () => {}
Element.prototype.scrollIntoView ??= () => {}
globalThis.IntersectionObserver ??= class {
  observe() {}
  disconnect() {}
  unobserve() {}
} as unknown as typeof IntersectionObserver

afterEach(() => vi.clearAllMocks())

const asset = (id: string, filename: string, over: object = {}) => ({
  __typename: 'MediaAsset' as const, id, path: `promo/${filename}`, directoryPath: 'promo', filename, url: `https://cdn/${filename}`,
  mimeType: 'image/png', mimeGroup: 'IMAGE' as const, sizeBytes: 2048, width: 40, height: 40, status: 'READY' as 'READY' | 'PENDING',
  title: '', altText: '', caption: '', notes: '', focalX: null, focalY: null, createdAt: '2026-10-01T00:00:00Z', updatedAt: '2026-10-01T00:00:00Z',
  ...over,
})

type Pending = { name: string; path: string; progress: (f: number) => void; land: (id: string) => Promise<void> }

function rig(props: Partial<Parameters<typeof MediaPickerDialog>[0]> = {}, landed: Record<string, ReturnType<typeof asset>> = {}, { refreshFails = false } = {}) {
  const pending: Pending[] = []
  vi.mocked(useMediaUpload).mockImplementation(
    (path: string) =>
      ((file: File, progress: (f: number) => void) =>
        new Promise((resolve) => {
          pending.push({ name: file.name, path, progress, land: async (id) => resolve({ id, url: `https://cdn/${id}` }) })
        })) as unknown as ReturnType<typeof useMediaUpload>,
  )
  const asked = { searches: 0, dirs: 0 }
  const onSelect = vi.fn()
  const onSelectMany = vi.fn()
  renderDialog(
    <MemoryRouter>
      <MediaPickerDialog open onOpenChange={() => {}} onSelect={onSelect} onSelectMany={onSelectMany} {...props} />
    </MemoryRouter>,
    [
      meMock(),
      {
        request: { query: AllMediaDirectoriesDocument },
        result: () => {
          asked.dirs++
          return { data: { allMediaDirectories: [{ __typename: 'MediaDirectoryChild' as const, path: 'promo', name: 'promo' }] } }
        },
        maxUsageCount: 10,
      },
      {
        request: { query: SearchMediaAssetsDocument, variables: () => true },
        result: () => {
          asked.searches++
          return { data: { searchMediaAssets: { __typename: 'MediaAssetSearchResult' as const, totalCount: 1, items: [asset('a1', 'red.png')] } } }
        },
        maxUsageCount: refreshFails ? 1 : 20,
      },
      ...(refreshFails ? [{ request: { query: SearchMediaAssetsDocument, variables: () => true }, error: new Error('Network down'), maxUsageCount: 5 }] : []),
      ...Object.entries(landed).map(([id, a]) => ({ request: { query: MediaAssetByIdDocument, variables: { id } }, result: { data: { mediaAsset: a } } })),
    ],
  )
  return { pending, asked, onSelect, onSelectMany }
}

const png = (name: string) => new File(['x'], name, { type: 'image/png' })
const ready = async () => {
  await screen.findByRole('option', { name: 'red.png' }, SLOW)
  // uploads wait for the grants, and so does the button
  await screen.findByRole('button', { name: 'Upload' }, SLOW)
}
const choose = (...files: File[]) => fireEvent.change(screen.getByLabelText('Upload files'), { target: { files } })
const picked = () => within(screen.getByRole('listbox', { name: 'Media' })).queryAllByRole('option').filter((o) => o.getAttribute('aria-selected') === 'true')

describe('media picker uploads', () => {
  it("goes into the caller's folder, shows progress, holds Use, then picks what landed", async () => {
    const { pending, asked, onSelect } = rig({ uploadPath: 'option-pictures' }, { new1: asset('new1', 'mug.png', { directoryPath: 'option-pictures' }) })
    await ready()
    fireEvent.click(screen.getByRole('option', { name: 'red.png' }))
    choose(png('mug.png'))
    await vi.waitFor(() => expect(pending).toHaveLength(1), SLOW)
    expect(pending[0].path).toBe('option-pictures')
    expect(screen.getByRole('button', { name: 'Use picture' })).toHaveProperty('disabled', true)

    act(() => pending[0].progress(0.5))
    expect(screen.getByRole('progressbar', { name: 'Uploading mug.png' }).getAttribute('aria-valuenow')).toBe('50')

    const before = { ...asked }
    await act(() => pending[0].land('new1'))
    expect(await within(screen.getByRole('list', { name: 'Uploads' })).findByText('Added', undefined, SLOW)).toBeDefined()
    await vi.waitFor(() => expect(screen.getByRole('complementary', { name: 'Details' }).textContent).toContain('mug.png'), SLOW)
    // the list and the folders refresh on the side, not before the pick
    await vi.waitFor(() => expect([asked.searches > before.searches, asked.dirs > before.dirs]).toEqual([true, true]), SLOW)

    fireEvent.click(screen.getByRole('button', { name: 'Use picture' }))
    expect(onSelect).toHaveBeenCalledWith(expect.objectContaining({ id: 'new1' }))
  })

  it('still picks what landed when the list fails to refresh', async () => {
    const { pending, onSelect } = rig({}, { new1: asset('new1', 'mug.png') }, { refreshFails: true })
    await ready()
    choose(png('mug.png'))
    await vi.waitFor(() => expect(pending).toHaveLength(1), SLOW)
    await act(() => pending[0].land('new1'))
    await vi.waitFor(() => expect(screen.getByRole('button', { name: 'Use picture' })).toHaveProperty('disabled', false), SLOW)
    fireEvent.click(screen.getByRole('button', { name: 'Use picture' }))
    expect(onSelect).toHaveBeenCalledWith(expect.objectContaining({ id: 'new1' }))
  })

  it('goes into the open folder when there is one', async () => {
    const { pending } = rig({ uploadPath: 'option-pictures' })
    await ready()
    fireEvent.click(await within(screen.getByRole('complementary', { name: 'Folders' })).findByText('promo', undefined, SLOW))
    choose(png('mug.png'))
    await vi.waitFor(() => expect(pending.map((p) => p.path)).toEqual(['promo']), SLOW)
  })

  it('adds what landed to the picks when it takes several', async () => {
    const { pending, onSelectMany } = rig({ multiple: true }, { n1: asset('n1', 'one.png'), n2: asset('n2', 'two.png') })
    await ready()
    fireEvent.click(screen.getByRole('option', { name: 'red.png' }))
    choose(png('one.png'), png('two.png'))
    await vi.waitFor(() => expect(pending).toHaveLength(2), SLOW)
    await act(async () => {
      await pending[0].land('n1')
      await pending[1].land('n2')
    })
    const use = await screen.findByRole('button', { name: 'Use 3 files' }, SLOW)
    fireEvent.click(use)
    expect(onSelectMany).toHaveBeenCalledWith([expect.objectContaining({ id: 'a1' }), expect.objectContaining({ id: 'n1' }), expect.objectContaining({ id: 'n2' })])
  })

  it("doesn't pick an upload the server hasn't finished", async () => {
    const { pending } = rig({}, { slow: asset('slow', 'huge.png', { status: 'PENDING' }) })
    await ready()
    choose(png('huge.png'))
    await vi.waitFor(() => expect(pending).toHaveLength(1), SLOW)
    await act(() => pending[0].land('slow'))
    expect(await within(screen.getByRole('list', { name: 'Uploads' })).findByText('Added', undefined, SLOW)).toBeDefined()
    expect(picked()).toEqual([])
    expect(screen.getByRole('button', { name: 'Use picture' })).toHaveProperty('disabled', true)
  })

  it('takes a dropped file and says where it will go while dragging', async () => {
    const { pending } = rig({ uploadPath: 'cms/pages' })
    await ready()
    const area = screen.getByRole('region', { name: 'Media' }).closest('[class*="relative flex"]') as HTMLElement
    fireEvent.dragEnter(area, { dataTransfer: { types: ['Files'], files: [] } })
    expect(screen.getByText('Drop to upload to cms/pages')).toBeDefined()
    fireEvent.drop(area, { dataTransfer: { types: ['Files'], files: [png('drop.png')] } })
    expect(screen.queryByText('Drop to upload to cms/pages')).toBeNull()
    await vi.waitFor(() => expect(pending.map((p) => [p.name, p.path])).toEqual([['drop.png', 'cms/pages']]), SLOW)
  })

  it('takes a pasted picture', async () => {
    const { pending } = rig()
    await ready()
    fireEvent.paste(screen.getByRole('textbox', { name: 'Search media' }), { clipboardData: { files: [png('clip.png')] } })
    await vi.waitFor(() => expect(pending.map((p) => [p.name, p.path])).toEqual([['clip.png', 'uploads']]), SLOW)
  })

  it('leaves a paste of plain text to the search box', async () => {
    const { pending } = rig()
    await ready()
    fireEvent.paste(screen.getByRole('textbox', { name: 'Search media' }), { clipboardData: { files: [] } })
    await new Promise((r) => setTimeout(r, 20))
    expect(pending).toEqual([])
  })
})
