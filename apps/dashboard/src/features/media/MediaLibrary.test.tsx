import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import type { MockLink } from '@apollo/client/testing'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { SLOW, meMock, renderPage } from '@/test/page-harness'
import { MediaLibrary } from './MediaLibrary'
import {
  AllMediaDirectoriesDocument,
  DeleteMediaAssetDocument,
  MediaAiStatusDocument,
  MediaCountsDocument,
  MediaDirectoryDocument,
  SearchMediaAssetsDocument,
} from './MediaLibrary.gql'

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
  ]
}

const library = (mocks = reads(), path = '/media?path=shoes') => renderPage({ path, adminId: 'admin-1', routes: { '/media': <MediaLibrary /> }, mocks })
const tile = (name: string) => screen.findByRole('option', { name }, SLOW)

afterEach(() => localStorage.clear())

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

    expect(screen.getByRole('radio', { name: /^Everything/ }).textContent).toContain('4')
    expect(screen.getByRole('radio', { name: /^Images/ }).textContent).toContain('3')
    expect(screen.getByRole('radio', { name: /^Documents/ }).textContent).toContain('1')
    expect(screen.getByText('4 files in shoes, its folders included')).toBeDefined()
    expect(within(screen.getByRole('list', { name: 'Folders here' })).getByRole('button', { name: /red/ })).toBeDefined()
  })

  it('asks the server for a kind and for words, and can go back to everything', async () => {
    const asked: Vars[] = []
    library(reads(WALL, asked))
    await tile('boot.jpg')
    expect(asked.at(-1)).toMatchObject({ path: 'shoes', query: null, mimeGroup: null, sort: 'NEWEST' })

    fireEvent.click(screen.getByRole('radio', { name: /^Images/ }))
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
    fireEvent.click(screen.getByRole('radio', { name: /^Images/ }))
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
