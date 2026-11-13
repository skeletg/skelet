import { act, fireEvent, screen, waitFor, within } from '@testing-library/react'
import type { MockLink } from '@apollo/client/testing'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { SLOW, meMock, renderPage } from '@/test/page-harness'
import { MediaLibrary } from './MediaLibrary'
import {
  AllMediaDirectoriesDocument,
  CreateMediaDirectoryDocument,
  DeleteMediaAssetDocument,
  DeleteMediaDirectoryDocument,
  MediaAiStatusDocument,
  MediaCountsDocument,
  MediaDirectoryDocument,
  MediaUploadTypesDocument,
  MoveMediaAssetDocument,
  RenameMediaDirectoryDocument,
  SearchMediaAssetsDocument,
} from './MediaLibrary.gql'
import { useMediaUpload } from './useImageUpload'

vi.mock('./useImageUpload', () => ({ useMediaUpload: vi.fn() }))

Element.prototype.hasPointerCapture ??= () => false
Element.prototype.releasePointerCapture ??= () => {}
Element.prototype.scrollIntoView ??= () => {}
globalThis.IntersectionObserver ??= class {
  observe() {}
  disconnect() {}
  unobserve() {}
} as unknown as typeof IntersectionObserver

const dirs = [
  { __typename: 'MediaDirectoryChild', path: 'shoes', name: 'shoes' },
  { __typename: 'MediaDirectoryChild', path: 'shoes/red', name: 'red' },
  { __typename: 'MediaDirectoryChild', path: 'hats', name: 'hats' },
]

const asset = (id: string, over: Record<string, unknown> = {}) => ({
  __typename: 'MediaAsset',
  id,
  path: `shoes/${id}.jpg`,
  directoryPath: 'shoes',
  filename: `${id}.jpg`,
  url: `/files/shoes/${id}.jpg`,
  thumbnailUrl: `/files/small/${id}.jpg`,
  mimeType: 'image/jpeg',
  mimeGroup: 'IMAGE',
  sizeBytes: 2048,
  width: 1200,
  height: 800,
  status: 'READY',
  title: '',
  altText: '',
  caption: '',
  notes: '',
  focalX: null,
  focalY: null,
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
  ...over,
})

const WALL = [asset('boot'), asset('sandal', { width: 600, height: 900 }), asset('logo', { width: 3000, height: 400 }), asset('manual', { mimeType: 'application/pdf', mimeGroup: 'DOCUMENT', width: null, height: null, filename: 'manual.pdf' })]

type Vars = Record<string, unknown>

function reads(items: object[] = WALL, asked: Vars[] = []): MockLink.MockedResponse[] {
  return [
    meMock(),
    { request: { query: AllMediaDirectoriesDocument }, result: { data: { allMediaDirectories: dirs } }, maxUsageCount: 20 },
    {
      request: { query: MediaDirectoryDocument, variables: () => true },
      result: (v: Vars) => ({
        data: { mediaDirectory: { __typename: 'MediaDirectory', path: v.path, name: v.path, directories: v.path === 'shoes' ? [dirs[1]] : [], assets: [], untrackedFiles: [] } },
      }),
      maxUsageCount: 20,
    },
    {
      request: { query: MediaCountsDocument, variables: () => true },
      result: { data: { all: { __typename: 'MediaAssetSearchResult', totalCount: 4 }, images: { __typename: 'MediaAssetSearchResult', totalCount: 3 }, documents: { __typename: 'MediaAssetSearchResult', totalCount: 1 } } },
      maxUsageCount: 20,
    },
    {
      request: {
        query: SearchMediaAssetsDocument,
        variables: (v: Vars) => {
          asked.push(v)
          return true
        },
      },
      result: { data: { searchMediaAssets: { __typename: 'MediaAssetSearchResult', totalCount: items.length, items } } },
      maxUsageCount: 40,
    },
    { request: { query: MediaAiStatusDocument }, result: { data: { aiStatus: { __typename: 'AiStatus', enabled: false } } }, maxUsageCount: 20 },
    { request: { query: MediaUploadTypesDocument }, result: { data: { mediaUploadTypes: ['image/jpeg', 'image/svg+xml', 'video/mp4', 'application/pdf'] } }, maxUsageCount: 20 },
  ]
}

const library = (mocks = reads(), path = '/media?path=shoes') => renderPage({ path, adminId: 'admin-1', routes: { '/media': <MediaLibrary /> }, mocks })
const tile = (name: string) => screen.findByRole('option', { name }, SLOW)
const details = () => screen.findByRole('complementary', { name: 'Details' }, SLOW)

// Uploads that wait to be landed by hand, so a test decides what the page is doing when one finishes
function uploadsByHand() {
  const waiting: { name: string; land: (id: string) => void }[] = []
  vi.mocked(useMediaUpload).mockImplementation(
    () => ((file: File) => new Promise((resolve) => waiting.push({ name: file.name, land: (id) => resolve({ id, url: `/files/${id}` }) }))) as unknown as ReturnType<typeof useMediaUpload>,
  )
  return waiting
}
const choose = (...names: string[]) =>
  fireEvent.change(screen.getByLabelText('Upload files'), { target: { files: names.map((n) => new File(['x'], n, { type: 'image/png' })) } })

afterEach(() => {
  localStorage.clear()
  vi.clearAllMocks()
})

// What this pins: the folder tree works from the keyboard - it was mouse-only rows. One tab stop, on the open folder,
// and Enter opens the focused one
describe('MediaLibrary folder tree', () => {
  it('tabs to the open folder and opens another with Enter', async () => {
    library()
    const tree = await screen.findByRole('tree', { name: 'Folders' })
    const shoes = await within(tree).findByRole('treeitem', { name: /shoes/ }, SLOW)
    const tabbable = within(tree).getAllByRole('treeitem').filter((r) => r.tabIndex === 0)
    expect(tabbable).toEqual([shoes])
    expect(shoes.getAttribute('aria-expanded')).toBe('true')

    fireEvent.keyDown(within(tree).getByRole('treeitem', { name: /hats/ }), { key: 'Enter' })
    await waitFor(() => expect(within(tree).getByRole('treeitem', { name: /hats/ }).getAttribute('aria-selected')).toBe('true'))
  })
})

describe('MediaLibrary wall', () => {
  it('hangs each file at its own shape, counts each kind, and offers the folders inside', async () => {
    library()

    const boot = await tile('boot.jpg')
    expect(boot.style.flexGrow).toBe('1.5')
    expect((await tile('sandal.jpg')).style.flexGrow).toBe(String(600 / 900))
    expect((await tile('logo.jpg')).style.flexGrow).toBe('2.4')
    expect((await tile('manual.pdf')).style.flexGrow).toBe('1')

    expect(screen.getByRole('button', { name: /^Everything/ }).textContent).toContain('4')
    expect(screen.getByRole('button', { name: /^Images/ }).textContent).toContain('3')
    expect(screen.getByRole('button', { name: /^Documents/ }).textContent).toContain('1')
    expect(screen.getByText('4 files in shoes, its folders included')).toBeDefined()
    expect(within(screen.getByRole('list', { name: 'Folders here' })).getByRole('button', { name: /red/ })).toBeDefined()
  })

  it('hangs the small copy of each picture, not the file itself', async () => {
    library()
    const img = within(await tile('boot.jpg')).getByRole('presentation') as HTMLImageElement
    expect(img.src).toContain('/files/small/boot.jpg')
    fireEvent.error(img)
    expect(img.src).toContain('/files/shoes/boot.jpg')
  })

  it('asks the server for a kind and for words, and can go back to everything', async () => {
    const asked: Vars[] = []
    library(reads(WALL, asked))
    await tile('boot.jpg')
    expect(asked.at(-1)).toMatchObject({ path: 'shoes', query: null, mimeGroup: null, sort: 'NEWEST' })

    fireEvent.click(screen.getByRole('button', { name: /^Images/ }))
    await waitFor(() => expect(asked.at(-1)).toMatchObject({ mimeGroup: 'IMAGE' }))

    fireEvent.change(screen.getByRole('searchbox', { name: 'Search media' }), { target: { value: 'red sole' } })
    await waitFor(() => expect(asked.at(-1)).toMatchObject({ query: 'red sole', mimeGroup: 'IMAGE' }), SLOW)
  })

  it('says a folder is empty and how to fill it', async () => {
    library(reads([]))
    expect(await screen.findByText('This folder is empty', {}, SLOW)).toBeDefined()
    expect(screen.getByText('Drop files here, paste a picture, or upload from your computer.')).toBeDefined()
  })

  it('shows where a dragged file will land, for someone who may upload', async () => {
    library()
    const wall = (await tile('boot.jpg')).closest('.relative') as HTMLElement
    fireEvent.dragEnter(wall, { dataTransfer: { types: ['Files'], files: [] } })
    expect(await screen.findByText('Drop to add to shoes')).toBeDefined()
  })

  it('offers no upload and no drop target to someone who may only look', async () => {
    library([meMock({ superuser: false, permissions: ['media.view'] }), ...reads().slice(1)])
    const wall = (await tile('boot.jpg')).closest('.relative') as HTMLElement
    fireEvent.dragEnter(wall, { dataTransfer: { types: ['Files'], files: [] } })
    expect(screen.queryByText('Drop to add to shoes')).toBeNull()
    expect(screen.queryByRole('button', { name: 'Upload' })).toBeNull()
    expect(screen.queryByRole('button', { name: 'New folder' })).toBeNull()
  })
})

describe('MediaLibrary picks', () => {
  it('opens a picked file beside the wall, and Ctrl picks several to act on together', async () => {
    library()

    fireEvent.click(await tile('boot.jpg'))
    const details = await screen.findByRole('complementary', { name: 'Details' })
    expect(within(details).getByRole('heading', { name: 'boot.jpg' })).toBeDefined()
    expect(within(details).getByLabelText('File name')).toHaveProperty('value', 'boot.jpg')
    expect(screen.queryByRole('toolbar', { name: 'Picked files' })).toBeNull()

    fireEvent.click(await tile('sandal.jpg'), { ctrlKey: true })
    expect(screen.queryByRole('complementary', { name: 'Details' })).toBeNull()
    expect(within(screen.getByRole('toolbar', { name: 'Picked files' })).getByText('2 picked')).toBeDefined()

    fireEvent.click(await tile('logo.jpg'))
    expect(within(await screen.findByRole('complementary', { name: 'Details' })).getByRole('heading', { name: 'logo.jpg' })).toBeDefined()
    fireEvent.click(within(screen.getByRole('complementary', { name: 'Details' })).getByRole('button', { name: 'Close details' }))
    expect(screen.queryByRole('complementary', { name: 'Details' })).toBeNull()
  })

  it('asks before dropping words typed for a file, and keeps them if told to', async () => {
    library()

    fireEvent.click(await tile('boot.jpg'))
    const alt = await screen.findByLabelText('Alt text', {}, SLOW)
    fireEvent.change(alt, { target: { value: 'A brown leather boot' } })

    fireEvent.click(await tile('sandal.jpg'))
    const ask = await screen.findByRole('alertdialog')
    expect(within(ask).getByText('Discard changes to this file?')).toBeDefined()
    fireEvent.click(within(ask).getByRole('button', { name: 'Cancel' }))
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
    expect(within(screen.getByRole('complementary', { name: 'Details' })).getByRole('heading', { name: 'boot.jpg' })).toBeDefined()
    expect((screen.getByLabelText('Alt text') as HTMLTextAreaElement).value).toBe('A brown leather boot')

    fireEvent.click(await tile('sandal.jpg'))
    fireEvent.click(within(await screen.findByRole('alertdialog')).getByRole('button', { name: 'Discard' }))
    expect(await within(screen.getByRole('complementary', { name: 'Details' })).findByRole('heading', { name: 'sandal.jpg' })).toBeDefined()
  })

  it('keeps the open file open when the wall turns to another kind', async () => {
    library()
    fireEvent.click(await tile('manual.pdf'))
    await screen.findByRole('complementary', { name: 'Details' })
    fireEvent.click(screen.getByRole('button', { name: /^Images/ }))
    expect(within(screen.getByRole('complementary', { name: 'Details' })).getByRole('heading', { name: 'manual.pdf' })).toBeDefined()
  })

  it('deletes the picked files once confirmed, and says how many it could not', async () => {
    const deleted = vi.fn()
    library([
      ...reads(),
      { request: { query: DeleteMediaAssetDocument, variables: { id: 'boot', deleteFromStorage: true } }, result: () => (deleted('boot'), { data: { deleteMediaAsset: true } }) },
      { request: { query: DeleteMediaAssetDocument, variables: { id: 'sandal', deleteFromStorage: true } }, error: new Error('still on a product page') },
    ])

    fireEvent.click(await tile('boot.jpg'))
    fireEvent.click(await tile('sandal.jpg'), { ctrlKey: true })
    fireEvent.click(within(screen.getByRole('toolbar', { name: 'Picked files' })).getByRole('button', { name: 'Delete' }))
    const ask = await screen.findByRole('alertdialog')
    expect(within(ask).getByText('Delete 2 files?')).toBeDefined()
    expect(deleted).not.toHaveBeenCalled()
    fireEvent.click(within(ask).getByRole('button', { name: 'Delete files' }))

    expect(await screen.findByText("1 of 2 couldn't be deleted: still on a product page", {}, SLOW)).toBeDefined()
    expect(deleted).toHaveBeenCalledWith('boot')
    await waitFor(() => expect(screen.queryByRole('option', { name: 'boot.jpg' })).toBeNull())
    expect(within(await details()).getByRole('heading', { name: 'sandal.jpg' })).toBeDefined()
  })

  it('moves the picked files, and the ones that left this folder leave the wall', async () => {
    const asked: Vars[] = []
    const moved = (id: string) => ({
      request: { query: MoveMediaAssetDocument, variables: { id, targetPath: 'hats', filename: `${id}.jpg` } },
      result: { data: { moveMediaAsset: asset(id, { directoryPath: 'hats', path: `hats/${id}.jpg` }) } },
    })
    library([...reads(WALL, asked), moved('boot'), moved('sandal')])

    fireEvent.click(await tile('boot.jpg'))
    fireEvent.click(await tile('sandal.jpg'), { ctrlKey: true })
    const searches = asked.length
    fireEvent.pointerDown(within(screen.getByRole('toolbar', { name: 'Picked files' })).getByRole('button', { name: 'Move to' }), { button: 0, ctrlKey: false })
    fireEvent.click(await screen.findByRole('menuitem', { name: 'hats' }))

    await waitFor(() => expect(screen.queryByRole('option', { name: 'boot.jpg' })).toBeNull(), SLOW)
    expect(screen.queryByRole('option', { name: 'sandal.jpg' })).toBeNull()
    expect(await tile('logo.jpg')).toBeDefined()
    expect(screen.getByText('2 files in shoes, its folders included')).toBeDefined()
    expect(asked.length).toBe(searches)
  })

  it('walks the wall with the arrows, and Enter opens where they land', async () => {
    library()
    const boot = await tile('boot.jpg')
    fireEvent.keyDown(boot, { key: 'ArrowRight' })
    expect(document.activeElement).toBe(await tile('sandal.jpg'))
    fireEvent.keyDown(document.activeElement!, { key: 'End' })
    expect(document.activeElement).toBe(await tile('manual.pdf'))
    fireEvent.keyDown(document.activeElement!, { key: 'Enter' })
    expect(within(await screen.findByRole('complementary', { name: 'Details' })).getByRole('heading', { name: 'manual.pdf' })).toBeDefined()
  })

  it('shows the wall as a list when asked, and remembers it', async () => {
    library()
    await tile('boot.jpg')
    fireEvent.click(screen.getByRole('button', { name: 'List' }))
    expect(screen.queryByLabelText('Picture size')).toBeNull()
    const row = (await screen.findByText('boot.jpg', {}, SLOW)).closest<HTMLElement>('[role=option]')
    expect(row?.style.flexGrow).toBe('')
    expect(row?.textContent).toContain('JPEG')
    expect(JSON.parse(localStorage.getItem('media-library:view') ?? '{}')).toMatchObject({ view: 'list' })
  })
})

// What these pin: the docked panel files one picture - rename, another folder, delete - and the wall follows
// without going back to the server for its first page, which would throw away how far it had been scrolled
describe('MediaLibrary one file', () => {
  it('renames a file where it hangs', async () => {
    library([
      ...reads(),
      {
        request: { query: MoveMediaAssetDocument, variables: { id: 'boot', targetPath: 'shoes', filename: 'chelsea.jpg' } },
        result: { data: { moveMediaAsset: asset('boot', { filename: 'chelsea.jpg', path: 'shoes/chelsea.jpg' }) } },
      },
    ])
    fireEvent.click(await tile('boot.jpg'))
    const panel = await details()
    fireEvent.change(within(panel).getByLabelText('File name'), { target: { value: 'chelsea.jpg' } })
    fireEvent.click(within(panel).getByRole('button', { name: 'Rename' }))

    expect(await within(panel).findByRole('heading', { name: 'chelsea.jpg' }, SLOW)).toBeDefined()
    expect(await tile('chelsea.jpg')).toBeDefined()
    expect(within(panel).getByRole('button', { name: 'Rename' })).toHaveProperty('disabled', true)
  })

  it('deletes a file and takes it off the wall without asking for the wall again', async () => {
    const asked: Vars[] = []
    library([...reads(WALL, asked), { request: { query: DeleteMediaAssetDocument, variables: { id: 'boot', deleteFromStorage: true } }, result: { data: { deleteMediaAsset: true } } }])
    fireEvent.click(await tile('boot.jpg'))
    const searches = asked.length
    fireEvent.click(within(await details()).getByRole('button', { name: 'Delete' }))
    fireEvent.click(within(await screen.findByRole('alertdialog')).getByRole('button', { name: 'Delete file' }))

    await waitFor(() => expect(screen.queryByRole('option', { name: 'boot.jpg' })).toBeNull(), SLOW)
    expect(screen.queryByRole('complementary', { name: 'Details' })).toBeNull()
    expect(screen.getByText('3 files in shoes, its folders included')).toBeDefined()
    expect(asked.length).toBe(searches)
  })

  it('says why a file could not be deleted and leaves it open', async () => {
    library([...reads(), { request: { query: DeleteMediaAssetDocument, variables: { id: 'boot', deleteFromStorage: true } }, error: new Error('storage is unreachable') }])
    fireEvent.click(await tile('boot.jpg'))
    fireEvent.click(within(await details()).getByRole('button', { name: 'Delete' }))
    fireEvent.click(within(await screen.findByRole('alertdialog')).getByRole('button', { name: 'Delete file' }))

    expect(await screen.findByText('storage is unreachable', {}, SLOW)).toBeDefined()
    expect(within(await details()).getByRole('heading', { name: 'boot.jpg' })).toBeDefined()
    expect(await tile('boot.jpg')).toBeDefined()
  })
})

describe('MediaLibrary folders', () => {
  it('makes a folder inside the open one', async () => {
    const made = vi.fn()
    library([
      ...reads(),
      { request: { query: CreateMediaDirectoryDocument, variables: { path: 'shoes', name: 'boots' } }, result: () => (made(), { data: { createMediaDirectory: { __typename: 'MediaDirectoryChild', path: 'shoes/boots', name: 'boots' } } }) },
    ])
    await tile('boot.jpg')
    fireEvent.click(within(screen.getByRole('complementary', { name: 'Folders' })).getByRole('button', { name: 'New folder' }))
    const dialog = await screen.findByRole('dialog')
    expect(within(dialog).getByText('It is made inside shoes.')).toBeDefined()
    fireEvent.change(within(dialog).getByLabelText('Folder name'), { target: { value: ' boots ' } })
    fireEvent.click(within(dialog).getByRole('button', { name: 'Create folder' }))

    await waitFor(() => expect(made).toHaveBeenCalled(), SLOW)
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  })

  it('renames the open folder and follows it to its new name', async () => {
    const asked: Vars[] = []
    library([
      ...reads(WALL, asked),
      { request: { query: RenameMediaDirectoryDocument, variables: { path: 'shoes', newName: 'footwear' } }, result: { data: { renameMediaDirectory: { __typename: 'MediaDirectoryChild', path: 'footwear', name: 'footwear' } } } },
    ])
    await tile('boot.jpg')
    fireEvent.pointerDown(screen.getByRole('button', { name: 'Folder' }), { button: 0, ctrlKey: false })
    fireEvent.click(await screen.findByRole('menuitem', { name: 'Rename' }))
    const dialog = await screen.findByRole('dialog')
    expect(within(dialog).getByLabelText('Folder name')).toHaveProperty('value', 'shoes')
    fireEvent.change(within(dialog).getByLabelText('Folder name'), { target: { value: 'footwear' } })
    fireEvent.click(within(dialog).getByRole('button', { name: 'Rename' }))

    await waitFor(() => expect(asked.at(-1)).toMatchObject({ path: 'footwear' }), SLOW)
  })

  it('asks before deleting a folder with everything in it, then steps up to its parent', async () => {
    const asked: Vars[] = []
    const deleted = vi.fn()
    library(
      [...reads(WALL, asked), { request: { query: DeleteMediaDirectoryDocument, variables: { path: 'shoes/red', recursive: true } }, result: () => (deleted(), { data: { deleteMediaDirectory: true } }) }],
      '/media?path=shoes/red',
    )
    await tile('boot.jpg')
    fireEvent.pointerDown(screen.getByRole('button', { name: 'Folder' }), { button: 0, ctrlKey: false })
    fireEvent.click(await screen.findByRole('menuitem', { name: 'Delete with everything in it' }))
    const ask = await screen.findByRole('alertdialog')
    expect(within(ask).getByText('Delete shoes/red and everything in it?')).toBeDefined()
    expect(deleted).not.toHaveBeenCalled()
    fireEvent.click(within(ask).getByRole('button', { name: 'Delete folder' }))

    await waitFor(() => expect(deleted).toHaveBeenCalled(), SLOW)
    await waitFor(() => expect(asked.at(-1)).toMatchObject({ path: 'shoes' }), SLOW)
  })

  it('says so when the folders could not be loaded', async () => {
    library([meMock(), { request: { query: AllMediaDirectoriesDocument }, error: new Error('folders are unreachable') }, ...reads().slice(2)])
    expect(await screen.findByText("The library couldn't be loaded", {}, SLOW)).toBeDefined()
    expect(screen.getByText('folders are unreachable')).toBeDefined()
  })
})

describe('MediaLibrary uploads', () => {
  it('offers the file dialog the types the server says it takes', async () => {
    library()
    await tile('boot.jpg')
    await waitFor(() => expect(screen.getByLabelText('Upload files').getAttribute('accept')).toBe('image/jpeg,image/svg+xml,video/mp4,application/pdf'), SLOW)
  })

  it('opens the file that just landed, and clears the finished ones together', async () => {
    const waiting = uploadsByHand()
    const wall = [...WALL]
    const search = { request: { query: SearchMediaAssetsDocument, variables: () => true }, result: () => ({ data: { searchMediaAssets: { __typename: 'MediaAssetSearchResult', totalCount: wall.length, items: [...wall] } } }), maxUsageCount: 40 }
    library([...reads().filter((m) => m.request.query !== SearchMediaAssetsDocument), search])
    await tile('boot.jpg')

    choose('heel.png', 'wedge.png')
    await waitFor(() => expect(waiting).toHaveLength(2))
    wall.unshift(asset('heel', { filename: 'heel.png' }), asset('wedge', { filename: 'wedge.png' }))
    await act(async () => {
      waiting[0]!.land('heel')
      waiting[1]!.land('wedge')
    })

    expect(await within(await details()).findByRole('heading', { name: 'wedge.png' }, SLOW)).toBeDefined()
    const list = screen.getByRole('list', { name: 'Uploads' })
    expect(within(list).getAllByText('Added')).toHaveLength(2)
    fireEvent.click(within(list).getByRole('button', { name: 'Clear the 2 finished' }))
    expect(screen.queryByRole('list', { name: 'Uploads' })).toBeNull()
  })

  it('leaves the open file open when an upload lands while its words are unsaved', async () => {
    const waiting = uploadsByHand()
    library()
    fireEvent.click(await tile('boot.jpg'))
    fireEvent.change(await screen.findByLabelText('Alt text', {}, SLOW), { target: { value: 'A brown leather boot' } })

    choose('heel.png')
    await waitFor(() => expect(waiting).toHaveLength(1))
    await act(async () => waiting[0]!.land('heel'))

    expect(await within(screen.getByRole('list', { name: 'Uploads' })).findByText('Added', {}, SLOW)).toBeDefined()
    expect(within(await details()).getByRole('heading', { name: 'boot.jpg' })).toBeDefined()
    expect((screen.getByLabelText('Alt text') as HTMLTextAreaElement).value).toBe('A brown leather boot')
    expect(screen.queryByRole('alertdialog')).toBeNull()
  })
})

describe('MediaLibrary paging', () => {
  it('offers the next page again when it could not be loaded', async () => {
    const page = (offset: number, items: object[]) => ({
      request: { query: SearchMediaAssetsDocument, variables: (v: Vars) => v.offset === offset },
      result: { data: { searchMediaAssets: { __typename: 'MediaAssetSearchResult', totalCount: 5, items } } },
      maxUsageCount: 5,
    })
    library([
      ...reads().filter((m) => m.request.query !== SearchMediaAssetsDocument),
      page(0, WALL),
      { request: { query: SearchMediaAssetsDocument, variables: (v: Vars) => v.offset === 4 }, error: new Error('Network down') },
      page(4, [asset('clog')]),
    ])
    await tile('boot.jpg')
    fireEvent.click(screen.getByRole('button', { name: 'Load more (4 of 5)' }))

    const again = await screen.findByRole('button', { name: "The rest couldn't be loaded. Try again" }, SLOW)
    expect(screen.queryByRole('option', { name: 'clog.jpg' })).toBeNull()
    fireEvent.click(again)
    expect(await tile('clog.jpg')).toBeDefined()
    expect(screen.queryByRole('button', { name: /Load more|Try again/ })).toBeNull()
  })
})
