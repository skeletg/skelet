import { fireEvent, screen, within } from '@testing-library/react'
import type { MockLink } from '@apollo/client/testing'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import { SLOW, meMock, renderDialog } from '@/test/page-harness'
import { AllMediaDirectoriesDocument, CreateMediaDirectoryDocument, MediaAssetByIdDocument, SearchMediaAssetsDocument } from '../MediaLibrary.gql'
import { MediaPickerDialog } from './MediaPickerDialog'

// What these pin: search, sort and folder reach the query, a click picks and Use hands it back, several pick in order
// and Shift picks a run, what's in use opens picked, the arrows and Enter work the grid, an unfinished upload can't be
// picked, a non-image is refused before upload, paging gets past the server's cap and stops when a page brings nothing,
// the empty states say which empty it is, a new folder opens, the view is remembered, and without media.manage it only picks

// Radix selects reach for these, and jsdom has none
Element.prototype.hasPointerCapture ??= () => false
Element.prototype.releasePointerCapture ??= () => {}
Element.prototype.scrollIntoView ??= () => {}
globalThis.IntersectionObserver ??= class {
  observe() {}
  disconnect() {}
  unobserve() {}
} as unknown as typeof IntersectionObserver

afterEach(() => localStorage.clear())

type Asset = ReturnType<typeof asset>
const asset = (id: string, filename: string, over: object = {}) => ({
  __typename: 'MediaAsset' as const, id, path: `promo/${filename}`, directoryPath: 'promo', filename, url: `https://cdn/${filename}`,
  mimeType: 'image/png', mimeGroup: 'IMAGE' as 'IMAGE' | 'DOCUMENT', sizeBytes: 2048, width: 40, height: 40, status: 'READY' as 'READY' | 'PENDING',
  title: '', altText: '', caption: '', notes: '', focalX: null, focalY: null, createdAt: '2026-10-01T00:00:00Z', updatedAt: '2026-10-01T00:00:00Z',
  ...over,
})
const assets = [asset('a1', 'red.png'), asset('a2', 'blue.png'), asset('a3', 'busy.png', { status: 'PENDING' })]
const dir = (path: string) => ({ __typename: 'MediaDirectoryChild' as const, path, name: path.split('/').at(-1) ?? path })

type Search = { query: string | null; path: string | null; mimeGroup: string | null; sort: string; limit: number; offset: number }

// A small stand-in for the server's search - folder is a prefix, words match the name, one call returns at most 200.
// `total` overrides the count, the way a delete between the count and the page leaves it
function rig({
  props = {},
  library = assets,
  dirs = [dir('promo')],
  me = meMock(),
  mocks = [],
  total,
}: {
  props?: Partial<Parameters<typeof MediaPickerDialog>[0]>
  library?: Asset[]
  dirs?: ReturnType<typeof dir>[]
  me?: MockLink.MockedResponse
  mocks?: MockLink.MockedResponse[]
  total?: number
} = {}) {
  const searches: Search[] = []
  const onSelect = vi.fn()
  const onSelectMany = vi.fn()
  const onOpenChange = vi.fn()
  const serve = (v: Search) => {
    searches.push(v)
    const hits = library.filter(
      (a) =>
        (!v.path || a.directoryPath === v.path || a.directoryPath.startsWith(`${v.path}/`)) &&
        (!v.query || a.filename.includes(v.query)) &&
        (!v.mimeGroup || a.mimeGroup === v.mimeGroup),
    )
    const items = hits.slice(v.offset, v.offset + Math.min(v.limit, 200))
    return { data: { searchMediaAssets: { __typename: 'MediaAssetSearchResult' as const, totalCount: total ?? hits.length, items } } }
  }
  renderDialog(
    <MemoryRouter>
      <MediaPickerDialog open onOpenChange={onOpenChange} onSelect={onSelect} onSelectMany={onSelectMany} {...props} />
    </MemoryRouter>,
    [
      me,
      { request: { query: AllMediaDirectoriesDocument }, result: { data: { allMediaDirectories: dirs } }, maxUsageCount: 10 },
      { request: { query: SearchMediaAssetsDocument, variables: () => true }, result: serve, maxUsageCount: 40 },
      ...mocks,
    ],
  )
  return { searches, onSelect, onSelectMany, onOpenChange }
}

const tiles = () => within(screen.getByRole('listbox', { name: 'Media' })).getAllByRole('option')
const tile = (name: string) => within(screen.getByRole('listbox', { name: 'Media' })).getByRole('option', { name })
// The folder list loads on its own query, so wait for the row
const openFolder = async (name: string) => fireEvent.click(await within(screen.getByRole('complementary', { name: 'Folders' })).findByText(name, undefined, SLOW))
const pickSelect = async (label: string, option: string) => {
  fireEvent.pointerDown(screen.getByRole('combobox', { name: label }), { button: 0, ctrlKey: false, pointerType: 'mouse' })
  fireEvent.click(await screen.findByRole('option', { name: option }))
}

describe('media picker', () => {
  it('picks one with a click and hands it back on Use', async () => {
    const { onSelect, onOpenChange } = rig()
    fireEvent.click(await screen.findByRole('option', { name: 'red.png' }, SLOW))
    expect(screen.getByRole('complementary', { name: 'Details' }).textContent).toContain('red.png')
    fireEvent.click(screen.getByRole('button', { name: 'Use picture' }))
    expect(onSelect).toHaveBeenCalledWith(expect.objectContaining({ id: 'a1' }))
    expect(onOpenChange).toHaveBeenCalledWith(false)
  })

  it('swaps the pick on another click when it takes one', async () => {
    const { onSelect } = rig()
    fireEvent.click(await screen.findByRole('option', { name: 'red.png' }, SLOW))
    fireEvent.click(tile('blue.png'))
    expect(tiles().filter((t) => t.getAttribute('aria-selected') === 'true')).toEqual([tile('blue.png')])
    fireEvent.click(screen.getByRole('button', { name: 'Use picture' }))
    expect(onSelect).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({ id: 'a2' }))
  })

  it('cancels without handing anything back', async () => {
    const { onSelect, onOpenChange } = rig()
    fireEvent.click(await screen.findByRole('option', { name: 'red.png' }, SLOW))
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(onOpenChange).toHaveBeenCalledWith(false)
    expect(onSelect).not.toHaveBeenCalled()
  })

  it("won't pick an upload that hasn't finished", async () => {
    rig()
    const busy = await screen.findByRole('option', { name: 'busy.png' }, SLOW)
    expect(busy.getAttribute('aria-disabled')).toBe('true')
    fireEvent.click(busy)
    expect(screen.getByRole('button', { name: 'Use picture' })).toHaveProperty('disabled', true)
  })

  it('picks several in order', async () => {
    const { onSelectMany } = rig({ props: { multiple: true } })
    fireEvent.click(await screen.findByRole('option', { name: 'blue.png' }, SLOW))
    fireEvent.click(tile('red.png'))
    fireEvent.click(screen.getByRole('button', { name: 'Use 2 files' }))
    expect(onSelectMany).toHaveBeenCalledWith([expect.objectContaining({ id: 'a2' }), expect.objectContaining({ id: 'a1' })])
  })

  it('takes a second click on a pick back, and clears them all', async () => {
    rig({ props: { multiple: true } })
    fireEvent.click(await screen.findByRole('option', { name: 'blue.png' }, SLOW))
    fireEvent.click(tile('red.png'))
    fireEvent.click(tile('blue.png'))
    expect(screen.getByText('1 picked')).toBeDefined()
    fireEvent.click(screen.getByRole('button', { name: 'Clear' }))
    expect(screen.getByText('Nothing picked yet')).toBeDefined()
    expect(screen.getByRole('button', { name: /^Use/ })).toHaveProperty('disabled', true)
  })

  it('picks a run with Shift, leaving out what cannot be picked', async () => {
    const library = [asset('a1', 'one.png'), asset('a2', 'two.png', { status: 'PENDING' }), asset('a3', 'three.png'), asset('a4', 'four.png')]
    const { onSelectMany } = rig({ props: { multiple: true }, library })
    fireEvent.click(await screen.findByRole('option', { name: 'one.png' }, SLOW))
    fireEvent.click(tile('three.png'), { shiftKey: true })
    fireEvent.click(screen.getByRole('button', { name: 'Use 2 files' }))
    expect(onSelectMany).toHaveBeenCalledWith([expect.objectContaining({ id: 'a1' }), expect.objectContaining({ id: 'a3' })])
  })

  it('opens with what is in use already picked, even off the first page', async () => {
    const elsewhere = asset('x9', 'old-logo.png', { directoryPath: 'archive' })
    const { onSelect } = rig({
      props: { selectedIds: ['x9'] },
      mocks: [{ request: { query: MediaAssetByIdDocument, variables: { id: 'x9' } }, result: { data: { mediaAsset: elsewhere } } }],
    })
    await screen.findByRole('option', { name: 'red.png' }, SLOW)
    await vi.waitFor(() => expect(screen.getByRole('button', { name: 'Use picture' })).toHaveProperty('disabled', false), SLOW)
    expect(screen.getByRole('complementary', { name: 'Details' }).textContent).toContain('old-logo.png')
    fireEvent.click(screen.getByRole('button', { name: 'Use picture' }))
    expect(onSelect).toHaveBeenCalledWith(expect.objectContaining({ id: 'x9' }))
  })

  it('shows what is in use picked in the grid', async () => {
    rig({ props: { selectedIds: ['a2'] }, mocks: [{ request: { query: MediaAssetByIdDocument, variables: { id: 'a2' } }, result: { data: { mediaAsset: assets[1] } } }] })
    const blue = await screen.findByRole('option', { name: 'blue.png' }, SLOW)
    expect(blue.getAttribute('aria-selected')).toBe('true')
    expect(blue.tabIndex).toBe(0)
  })

  it('moves with the arrows and uses one with Enter', async () => {
    const { onSelect } = rig()
    const first = await screen.findByRole('option', { name: 'red.png' }, SLOW)
    first.focus()
    fireEvent.keyDown(first, { key: 'ArrowRight' })
    expect(document.activeElement).toBe(tile('blue.png'))
    fireEvent.keyDown(document.activeElement as Element, { key: 'Enter' })
    expect(onSelect).toHaveBeenCalledWith(expect.objectContaining({ id: 'a2' }))
  })

  it('uses one on a double click', async () => {
    const { onSelect } = rig()
    fireEvent.doubleClick(await screen.findByRole('option', { name: 'blue.png' }, SLOW))
    expect(onSelect).toHaveBeenCalledWith(expect.objectContaining({ id: 'a2' }))
  })

  it('sends the search, the folder and the sort to the query', async () => {
    const { searches } = rig()
    await screen.findByRole('option', { name: 'red.png' }, SLOW)
    expect(searches[0]).toMatchObject({ query: null, path: null, sort: 'NEWEST', mimeGroup: 'IMAGE', limit: 48, offset: 0 })
    fireEvent.change(screen.getByRole('textbox', { name: 'Search media' }), { target: { value: ' red ' } })
    await openFolder('promo')
    await pickSelect('Sort', 'Name A-Z')
    await vi.waitFor(() => expect(searches.at(-1)).toMatchObject({ query: 'red', path: 'promo', sort: 'NAME', mimeGroup: 'IMAGE' }), SLOW)
    await openFolder('All media')
    await vi.waitFor(() => expect(searches.at(-1)).toMatchObject({ path: null }), SLOW)
  })

  it('picks a folder from the narrow-screen list too', async () => {
    const { searches } = rig({ dirs: [dir('promo'), dir('promo/autumn')] })
    await screen.findByRole('option', { name: 'red.png' }, SLOW)
    await screen.findByRole('option', { name: 'promo/autumn' }, SLOW)
    fireEvent.change(screen.getByRole('combobox', { name: 'Folder' }), { target: { value: 'promo/autumn' } })
    await vi.waitFor(() => expect(searches.at(-1)).toMatchObject({ path: 'promo/autumn' }), SLOW)
  })

  it('searches everything or one type when it takes any file', async () => {
    const library = [asset('a1', 'red.png'), asset('d1', 'terms.pdf', { mimeGroup: 'DOCUMENT', mimeType: 'application/pdf' })]
    const { searches, onSelect } = rig({ props: { accept: 'any' }, library })
    await screen.findByRole('option', { name: 'terms.pdf' }, SLOW)
    expect(searches[0]).toMatchObject({ mimeGroup: null })
    fireEvent.click(tile('terms.pdf'))
    fireEvent.click(screen.getByRole('button', { name: 'Use file' }))
    expect(onSelect).toHaveBeenCalledWith(expect.objectContaining({ id: 'd1' }))
    await pickSelect('Type', 'Images')
    await vi.waitFor(() => expect(searches.at(-1)).toMatchObject({ mimeGroup: 'IMAGE' }), SLOW)
  })

  it('asks a document picker for documents and keeps pictures out of it', async () => {
    const pdf = asset('d1', 'terms.pdf', { mimeGroup: 'DOCUMENT', mimeType: 'application/pdf' })
    const { searches } = rig({ props: { accept: 'document' }, library: [pdf] })
    await screen.findByRole('option', { name: 'terms.pdf' }, SLOW)
    expect(searches[0]).toMatchObject({ mimeGroup: 'DOCUMENT' })
    expect(screen.queryByRole('combobox', { name: 'Type' })).toBeNull()
    expect(tile('terms.pdf').getAttribute('aria-disabled')).toBe('false')
  })

  // The server answers with a picture anyway, to show the picker's own guard holds
  it("won't let a picture through a document picker", async () => {
    const red = asset('a1', 'red.png')
    renderDialog(
      <MemoryRouter>
        <MediaPickerDialog open onOpenChange={() => {}} accept="document" />
      </MemoryRouter>,
      [
        meMock(),
        { request: { query: AllMediaDirectoriesDocument }, result: { data: { allMediaDirectories: [] } }, maxUsageCount: 5 },
        {
          request: { query: SearchMediaAssetsDocument, variables: () => true },
          result: { data: { searchMediaAssets: { __typename: 'MediaAssetSearchResult', totalCount: 1, items: [red] } } },
          maxUsageCount: 5,
        },
      ],
    )
    const t = await screen.findByRole('option', { name: 'red.png' }, SLOW)
    expect([t.getAttribute('aria-disabled'), t.title]).toEqual(['true', 'Only documents can be used here'])
  })

  it('pages past the server cap of 200 a search can return', async () => {
    const library = Array.from({ length: 250 }, (_, i) => asset(`p${i}`, `pic-${String(i).padStart(3, '0')}.png`))
    const { searches } = rig({ library })
    await screen.findByRole('option', { name: 'pic-000.png' }, SLOW)
    for (let i = 0; i < 10; i++) {
      const more = screen.queryByRole('button', { name: /Load more/ })
      if (!more) break
      const before = tiles().length
      fireEvent.click(more)
      await vi.waitFor(() => expect(tiles().length, `stuck at ${before} after asking for more`).toBeGreaterThan(before), SLOW)
    }
    expect(tiles()).toHaveLength(250)
    expect(new Set(tiles().map((t) => t.getAttribute('data-asset'))).size).toBe(250)
    expect(Math.max(...searches.map((v) => v.limit))).toBeLessThanOrEqual(200)
  })

  it('stops asking for more when a page brings nothing new', async () => {
    const library = Array.from({ length: 48 }, (_, i) => asset(`p${i}`, `pic-${i}.png`))
    const { searches } = rig({ library, total: 60 })
    await screen.findByRole('option', { name: 'pic-0.png' }, SLOW)
    fireEvent.click(screen.getByRole('button', { name: 'Load more (48 of 60)' }))
    await vi.waitFor(() => expect(screen.queryByRole('button', { name: /Load more|Loading/ })).toBeNull(), SLOW)
    expect(searches.map((s) => s.offset)).toEqual([0, 48])
    expect(tiles()).toHaveLength(48)
  })

  it('says the search found nothing and clears it', async () => {
    const { searches } = rig()
    await screen.findByRole('option', { name: 'red.png' }, SLOW)
    fireEvent.change(screen.getByRole('textbox', { name: 'Search media' }), { target: { value: 'zebra' } })
    expect(await screen.findByText('Nothing matches', undefined, SLOW)).toBeDefined()
    fireEvent.click(screen.getByRole('button', { name: 'Clear the search' }))
    expect(await screen.findByRole('option', { name: 'red.png' }, SLOW)).toBeDefined()
    await vi.waitFor(() => expect(searches.at(-1)).toMatchObject({ query: null }), SLOW)
  })

  it('tells an empty folder from an empty library', async () => {
    rig({ library: [], dirs: [dir('promo')] })
    expect(await screen.findByText('The library is empty', undefined, SLOW)).toBeDefined()
    await openFolder('promo')
    expect(await screen.findByText('This folder is empty', undefined, SLOW)).toBeDefined()
    expect(screen.getAllByRole('button', { name: 'Upload' })).toHaveLength(2)
  })

  it('makes a new folder inside the open one and opens it', async () => {
    const made: Record<string, unknown>[] = []
    const { searches } = rig({
      mocks: [
        {
          request: {
            query: CreateMediaDirectoryDocument,
            variables: (v: Record<string, unknown>) => {
              made.push(v)
              return true
            },
          },
          result: { data: { createMediaDirectory: { __typename: 'MediaDirectoryChild', path: 'promo/autumn', name: 'autumn' } } },
        },
      ],
    })
    await screen.findByRole('option', { name: 'red.png' }, SLOW)
    await openFolder('promo')
    fireEvent.click(screen.getByRole('button', { name: 'New folder here' }))
    fireEvent.change(screen.getByRole('textbox', { name: 'New folder name' }), { target: { value: ' autumn ' } })
    fireEvent.click(screen.getByRole('button', { name: 'Add' }))
    await vi.waitFor(() => expect(searches.at(-1)).toMatchObject({ path: 'promo/autumn' }), SLOW)
    expect(made).toEqual([{ path: 'promo', name: 'autumn' }])
  })

  it('drops an unnamed new folder without asking the server', async () => {
    const asked = vi.fn(() => true)
    rig({ mocks: [{ request: { query: CreateMediaDirectoryDocument, variables: asked }, result: { data: { createMediaDirectory: dir('x') } } }] })
    await screen.findByRole('option', { name: 'red.png' }, SLOW)
    fireEvent.click(screen.getByRole('button', { name: 'New folder' }))
    fireEvent.change(screen.getByRole('textbox', { name: 'New folder name' }), { target: { value: '   ' } })
    fireEvent.click(screen.getByRole('button', { name: 'Add' }))
    expect(await screen.findByRole('button', { name: 'New folder' })).toBeDefined()
    expect(asked).not.toHaveBeenCalled()
  })

  it('remembers list view and tile size for the next time it opens', async () => {
    rig()
    await screen.findByRole('option', { name: 'red.png' }, SLOW)
    await pickSelect('Tile size', 'Small tiles')
    fireEvent.click(screen.getByRole('button', { name: 'List' }))
    expect(screen.getByRole('button', { name: 'List' }).getAttribute('aria-pressed')).toBe('true')
    expect(screen.queryByRole('combobox', { name: 'Tile size' })).toBeNull()
    expect(JSON.parse(localStorage.getItem('media-picker:view') ?? '{}')).toEqual({ view: 'list', size: 'small' })
  })

  it('opens in the view it was left in', async () => {
    localStorage.setItem('media-picker:view', JSON.stringify({ view: 'list', size: 'small' }))
    rig()
    await screen.findByRole('option', { name: /red\.png/ }, SLOW)
    expect(screen.getByRole('button', { name: 'List' }).getAttribute('aria-pressed')).toBe('true')
  })

  it('shrugs off a broken saved view', async () => {
    localStorage.setItem('media-picker:view', '{not json')
    rig()
    await screen.findByRole('option', { name: 'red.png' }, SLOW)
    expect(screen.getByRole('button', { name: 'Grid' }).getAttribute('aria-pressed')).toBe('true')
  })

  it('refuses a file that is not a picture before uploading it', async () => {
    rig()
    // Files are ignored until the grants arrive, and the Upload button waits for them too
    await screen.findByRole('button', { name: 'Upload' }, SLOW)
    fireEvent.change(screen.getByLabelText('Upload files'), { target: { files: [new File(['x'], 'notes.txt', { type: 'text/plain' })] } })
    expect(await screen.findByText('Not an image - only pictures can be used here', undefined, SLOW)).toBeDefined()
    fireEvent.click(screen.getByRole('button', { name: 'Dismiss notes.txt' }))
    expect(screen.queryByText('Not an image - only pictures can be used here')).toBeNull()
  })

  it('only picks without media.manage - no uploads, no new folders, drops ignored', async () => {
    const { onSelect } = rig({ me: meMock({ superuser: false, permissions: ['media.view'] }) })
    fireEvent.click(await screen.findByRole('option', { name: 'red.png' }, SLOW))
    expect(screen.queryByRole('button', { name: 'Upload' })).toBeNull()
    expect(screen.queryByRole('button', { name: /New folder/ })).toBeNull()
    fireEvent.change(screen.getByLabelText('Upload files'), { target: { files: [new File(['x'], 'notes.txt', { type: 'text/plain' })] } })
    expect(screen.queryByRole('list', { name: 'Uploads' })).toBeNull()
    expect(screen.getByLabelText('Alt text').matches(':disabled')).toBe(true)
    fireEvent.click(screen.getByRole('button', { name: 'Use picture' }))
    expect(onSelect).toHaveBeenCalledWith(expect.objectContaining({ id: 'a1' }))
  })
})
