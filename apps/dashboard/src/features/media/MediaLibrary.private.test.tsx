import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import type { MockLink } from '@apollo/client/testing'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { SLOW, meMock, renderPage } from '@/test/page-harness'
import { MediaLibrary } from './MediaLibrary'
import {
  AllMediaDirectoriesDocument,
  CreateMediaDirectoryDocument,
  MediaAiStatusDocument,
  MediaAssetSharesDocument,
  MediaCountsDocument,
  MediaDirectoryDocument,
  MediaPrivateAreaDocument,
  MediaUploadTypesDocument,
  MoveMediaAssetDocument,
  SearchMediaAssetsDocument,
} from './MediaLibrary.gql'
import { useMediaUpload } from './useImageUpload'


vi.mock('./useImageUpload', () => ({ useMediaUpload: vi.fn() }))
vi.mock('./editor/PhotoEditor', () => ({ default: () => null }))

Element.prototype.hasPointerCapture ??= () => false
Element.prototype.releasePointerCapture ??= () => {}
Element.prototype.scrollIntoView ??= () => {}
// Everything is on screen here, so a private tile asks for its picture as soon as it is drawn
globalThis.IntersectionObserver ??= class {
  seen: IntersectionObserverCallback
  constructor(seen: IntersectionObserverCallback) {
    this.seen = seen
  }
  observe(target: Element) {
    this.seen([{ isIntersecting: true, target } as IntersectionObserverEntry], this as unknown as IntersectionObserver)
  }
  disconnect() {}
  unobserve() {}
} as unknown as typeof IntersectionObserver

type Vars = Record<string, unknown>

const asset = (id: string, over: Vars = {}) => ({
  __typename: 'MediaAsset', id, path: `${id}.jpg`, directoryPath: '', filename: `${id}.jpg`, url: `/files/${id}.jpg`, thumbnailUrl: `/files/small/${id}.jpg`, visibility: 'PUBLIC',
  mimeType: 'image/jpeg', mimeGroup: 'IMAGE', sizeBytes: 2048, width: 1200, height: 800, status: 'READY',
  title: '', altText: '', caption: '', notes: '', focalX: null, focalY: null, createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-01T00:00:00Z',
  ...over,
})
const hidden = (id: string, over: Vars = {}) => asset(id, { visibility: 'PRIVATE', url: '', thumbnailUrl: '', ...over })
const folder = (path: string) => ({ __typename: 'MediaDirectoryChild', path, name: path })

function reads({ privateStore = true, asked = {} as Record<string, Vars[]>, kept = 'deed', shares = [] as object[] } = {}): MockLink.MockedResponse[] {
  const note = (op: string) => (v: Vars) => {
    ;(asked[op] ??= []).push(v)
    return true
  }
  const inPrivate = (v: Vars) => v.visibility === 'PRIVATE'
  return [
    meMock(),
    { request: { query: MediaPrivateAreaDocument, variables: note('store') }, result: { data: { mediaPrivateArea: privateStore } }, maxUsageCount: 20 },
    {
      request: { query: AllMediaDirectoriesDocument, variables: note('folders') },
      result: (v: Vars) => ({ data: { allMediaDirectories: [folder(inPrivate(v) ? 'contracts' : 'shoes')] } }),
      maxUsageCount: 20,
    },
    {
      request: { query: MediaDirectoryDocument, variables: note('directory') },
      result: (v: Vars) => ({ data: { mediaDirectory: { __typename: 'MediaDirectory', path: v.path, name: v.path, directories: [], assets: [], untrackedFiles: [] } } }),
      maxUsageCount: 20,
    },
    {
      request: { query: MediaCountsDocument, variables: note('counts') },
      result: { data: { all: { __typename: 'MediaAssetSearchResult', totalCount: 1 }, images: { __typename: 'MediaAssetSearchResult', totalCount: 1 }, documents: { __typename: 'MediaAssetSearchResult', totalCount: 0 } } },
      maxUsageCount: 20,
    },
    {
      request: { query: SearchMediaAssetsDocument, variables: note('search') },
      result: (v: Vars) => ({ data: { searchMediaAssets: { __typename: 'MediaAssetSearchResult', totalCount: 1, items: [inPrivate(v) ? hidden(kept) : asset('boot')] } } }),
      maxUsageCount: 40,
    },
    { request: { query: MediaAssetSharesDocument, variables: () => true }, result: { data: { mediaAssetShares: shares } }, maxUsageCount: 20 },
    { request: { query: MediaAiStatusDocument }, result: { data: { aiStatus: { __typename: 'AiStatus', enabled: false } } }, maxUsageCount: 20 },
    { request: { query: MediaUploadTypesDocument }, result: { data: { mediaUploadTypes: ['image/jpeg', 'application/pdf'] } }, maxUsageCount: 20 },
  ]
}

const library = (mocks = reads(), path = '/media') => renderPage({ path, adminId: 'admin-1', routes: { '/media': <MediaLibrary /> }, mocks })
const tile = (name: string) => screen.findByRole('option', { name }, SLOW)
const details = () => screen.findByRole('complementary', { name: 'Details' }, SLOW)
const halves = () => screen.findAllByRole('group', { name: 'Which files' }, SLOW)
const visibilities = (asked: Vars[] = []) => [...new Set(asked.map((v) => ('visibility' in v ? v.visibility : 'left out')))]

beforeEach(() => {
  URL.createObjectURL = vi.fn(() => 'blob:kept')
  vi.stubGlobal('fetch', vi.fn(async () => new Response(new Blob(['jpg']))))
})
afterEach(() => {
  localStorage.clear()
  vi.unstubAllGlobals()
  vi.clearAllMocks()
})

describe('MediaLibrary halves', () => {
  it('offers no private half on an install without a private store, even when the address asks for it', async () => {
    const asked: Record<string, Vars[]> = {}
    library(reads({ privateStore: false, asked }), '/media?area=private')
    expect(await tile('boot.jpg')).toBeDefined()
    expect(screen.queryByRole('group', { name: 'Which files' })).toBeNull()
    expect(screen.getByText('1 file in the library')).toBeDefined()
    expect(['folders', 'directory', 'counts', 'search'].flatMap((op) => visibilities(asked[op]))).toEqual(['left out', 'left out', 'left out', 'left out'])

    fireEvent.click(await tile('boot.jpg'))
    expect(within(await details()).queryByRole('button', { name: /Make private/ })).toBeNull()
  })

  it('opens on the public half and asks for it the way it always did', async () => {
    const asked: Record<string, Vars[]> = {}
    library(reads({ asked }))
    expect(await tile('boot.jpg')).toBeDefined()
    const [switcher] = await halves()
    expect(within(switcher!).getByRole('button', { name: 'Public' }).getAttribute('aria-pressed')).toBe('true')
    expect(within(screen.getByRole('tree', { name: 'Folders' })).getByRole('treeitem', { name: /shoes/ })).toBeDefined()
    expect(['folders', 'directory', 'counts', 'search'].flatMap((op) => visibilities(asked[op]))).toEqual(['left out', 'left out', 'left out', 'left out'])
  })

  it('switches to the private half, with its own folders and files and no search carried over', async () => {
    const asked: Record<string, Vars[]> = {}
    library(reads({ asked }), '/media?path=shoes&q=boot')
    expect(await tile('boot.jpg')).toBeDefined()
    const [switcher] = await halves()
    fireEvent.click(within(switcher!).getByRole('button', { name: 'Private' }))
    expect(screen.queryByRole('option', { name: 'boot.jpg' })).toBeNull()

    expect(await tile('deed.jpg')).toBeDefined()
    expect(await screen.findByText('1 private file in the library', {}, SLOW)).toBeDefined()
    const tree = screen.getByRole('tree', { name: 'Folders' })
    await within(tree).findByRole('treeitem', { name: /contracts/ }, SLOW)
    expect(within(tree).queryByRole('treeitem', { name: /shoes/ })).toBeNull()
    expect(within(tree).getByRole('treeitem', { name: /Private files/ })).toBeDefined()
    expect((screen.getByRole('searchbox') as HTMLInputElement).value).toBe('')

    for (const op of ['folders', 'directory', 'counts', 'search']) expect([op, asked[op]!.at(-1)!.visibility]).toEqual([op, 'PRIVATE'])
    expect(asked.search!.at(-1)).toMatchObject({ path: null, query: null })
  })

  it('opens straight onto the private half from its address, and shows its pictures through the API', async () => {
    const asked: Record<string, Vars[]> = {}
    library(reads({ kept: 'lease', asked }), '/media?area=private')
    const lease = await tile('lease.jpg')
    await waitFor(() => expect(lease.querySelector('img')?.getAttribute('src')).toBe('blob:kept'), SLOW)
    expect(vi.mocked(fetch).mock.calls.map((c) => c[0])).toEqual(['/api/admin/media/file/lease?w=640'])
    const [switcher] = await halves()
    expect(within(switcher!).getByRole('button', { name: 'Private' }).getAttribute('aria-pressed')).toBe('true')
    for (const op of ['folders', 'directory', 'counts', 'search']) expect([op, visibilities(asked[op])]).toEqual([op, ['PRIVATE']])
  })

  it('uploads into the half being looked at', async () => {
    vi.mocked(useMediaUpload).mockReturnValue((async () => ({ id: 'deed', url: '' })) as unknown as ReturnType<typeof useMediaUpload>)
    library(reads(), '/media?area=private')
    await tile('deed.jpg')
    expect(vi.mocked(useMediaUpload).mock.calls.at(-1)).toEqual(['', 'PRIVATE'])

    fireEvent.click(within((await halves())[0]!).getByRole('button', { name: 'Public' }))
    await tile('boot.jpg')
    expect(vi.mocked(useMediaUpload).mock.calls.at(-1)).toEqual(['', 'PUBLIC'])
  })

  it('makes a folder in the private half', async () => {
    const made: Vars[] = []
    library(
      [
        ...reads(),
        {
          request: { query: CreateMediaDirectoryDocument, variables: (v: Vars) => (made.push(v), true) },
          result: { data: { createMediaDirectory: { __typename: 'MediaDirectory', path: 'deeds', name: 'deeds' } } },
        },
      ],
      '/media?area=private',
    )
    await tile('deed.jpg')
    fireEvent.click(screen.getAllByRole('button', { name: 'New folder' })[0]!)
    const dialog = await screen.findByRole('dialog')
    fireEvent.change(within(dialog).getByRole('textbox'), { target: { value: 'deeds' } })
    fireEvent.click(within(dialog).getByRole('button', { name: 'Create folder' }))
    await waitFor(() => expect(made).toEqual([{ path: '', name: 'deeds', visibility: 'PRIVATE' }]), SLOW)
  })
})

describe('MediaLibrary crossing halves', () => {
  it('makes a file private once confirmed, and takes it off the public wall', async () => {
    const moved: Vars[] = []
    library([
      ...reads(),
      { request: { query: MoveMediaAssetDocument, variables: (v: Vars) => (moved.push(v), true) }, result: { data: { moveMediaAsset: hidden('boot') } } },
    ])
    fireEvent.click(await tile('boot.jpg'))
    fireEvent.click(within(await details()).getByRole('button', { name: /Make private/ }))
    const sure = await screen.findByRole('alertdialog')
    expect(within(sure).getByText(/loses its address/)).toBeDefined()
    expect(moved).toEqual([])
    fireEvent.click(within(sure).getByRole('button', { name: 'Make private' }))

    await waitFor(() => expect(screen.queryByRole('option', { name: 'boot.jpg' })).toBeNull(), SLOW)
    expect(screen.queryByRole('complementary', { name: 'Details' })).toBeNull()
    expect(moved).toEqual([{ id: 'boot', targetPath: '', filename: 'boot.jpg', visibility: 'PRIVATE' }])
  })

  it('makes a private file public, and shows who it is shared with until then', async () => {
    const moved: Vars[] = []
    library(
      [
        ...reads(),
        { request: { query: MoveMediaAssetDocument, variables: (v: Vars) => (moved.push(v), true) }, result: { data: { moveMediaAsset: asset('deed') } } },
      ],
      '/media?area=private',
    )
    fireEvent.click(await tile('deed.jpg'))
    const panel = await details()
    expect(await within(panel).findByRole('region', { name: 'Shared with' }, SLOW)).toBeDefined()
    fireEvent.click(within(panel).getByRole('button', { name: /Make public/ }))
    fireEvent.click(within(await screen.findByRole('alertdialog')).getByRole('button', { name: 'Make public' }))

    await waitFor(() => expect(screen.queryByRole('option', { name: 'deed.jpg' })).toBeNull(), SLOW)
    expect(moved).toEqual([{ id: 'deed', targetPath: '', filename: 'deed.jpg', visibility: 'PUBLIC' }])
  })

  it('says why a file could not change halves and leaves it where it was', async () => {
    library([...reads(), { request: { query: MoveMediaAssetDocument, variables: () => true }, error: new Error('the private store is unreachable') }])
    fireEvent.click(await tile('boot.jpg'))
    fireEvent.click(within(await details()).getByRole('button', { name: /Make private/ }))
    fireEvent.click(within(await screen.findByRole('alertdialog')).getByRole('button', { name: 'Make private' }))

    expect(await screen.findByText('the private store is unreachable', {}, SLOW)).toBeDefined()
    expect(await tile('boot.jpg')).toBeDefined()
    expect(within(await details()).getByRole('heading', { name: 'boot.jpg' })).toBeDefined()
  })

  it('still lists the shares of a file that was made public, so they can be taken back', async () => {
    const share = { __typename: 'MediaShare', id: 's1', customerId: 'c1', email: 'bo@example.com', downloadsUsed: 1, maxDownloads: null, expiresAt: null, createdAt: '2026-11-01T00:00:00Z' }
    library(reads({ shares: [share] }))
    fireEvent.click(await tile('boot.jpg'))
    const shared = await within(await details()).findByRole('region', { name: 'Shared with' }, SLOW)
    expect(within(shared).getByRole('button', { name: 'Stop sharing with bo@example.com' })).toBeDefined()
    expect(within(shared).queryByRole('button', { name: 'Share with a customer' })).toBeNull()
  })

  it('offers no shares on a public file, and no crossing to someone who may only look', async () => {
    library([meMock({ superuser: false, permissions: ['media.view'] }), ...reads().slice(1)])
    fireEvent.click(await tile('boot.jpg'))
    const panel = await details()
    expect(within(panel).queryByRole('region', { name: 'Shared with' })).toBeNull()
    expect(within(panel).queryByRole('button', { name: /Make private/ })).toBeNull()
  })
})

describe('MediaLibrary private half grants', () => {
  const as = (...permissions: string[]) => [meMock({ superuser: false, permissions }), ...reads().slice(1)]
  const upload = () => screen.queryByRole('button', { name: 'Upload' })

  it('offers no private half on the grants for the library alone, and never asks whether there is one', async () => {
    const asked: Record<string, Vars[]> = {}
    library([meMock({ superuser: false, permissions: ['media.manage'] }), ...reads({ asked }).slice(1)], '/media?area=private')
    fireEvent.click(await tile('boot.jpg'))
    expect(asked.store).toBeUndefined()
    const panel = await details()
    expect(screen.queryByRole('group', { name: 'Which files' })).toBeNull()
    expect(within(panel).getByLabelText('File name')).toBeDefined()
    expect(within(panel).queryByRole('button', { name: /Make private/ })).toBeNull()
    expect(within(panel).queryByRole('region', { name: 'Shared with' })).toBeNull()
  })

  it('lets someone who may see the private half look, and change nothing', async () => {
    library(as('media.view', 'media.private.view'), '/media?area=private')
    fireEvent.click(await tile('deed.jpg'))
    const panel = await details()
    expect(await within(panel).findByRole('region', { name: 'Shared with' }, SLOW)).toBeDefined()
    expect(within(panel).getByRole('button', { name: 'Download' })).toBeDefined()
    expect(upload()).toBeNull()
    expect(within(panel).queryByLabelText('File name')).toBeNull()
    expect(within(panel).queryByRole('button', { name: /Make public/ })).toBeNull()
    expect(within(panel).queryByRole('button', { name: 'Share with a customer' })).toBeNull()
  })

  it('keeps a manager of the public half from uploading to the private one or moving a file across', async () => {
    library(as('media.manage', 'media.private.view'), '/media?area=private')
    fireEvent.click(await tile('deed.jpg'))
    expect(upload()).toBeNull()
    expect(within(await details()).queryByLabelText('File name')).toBeNull()

    fireEvent.click(within((await halves())[0]!).getByRole('button', { name: 'Public' }))
    fireEvent.click(await tile('boot.jpg'))
    const panel = await details()
    expect(await within(panel).findByLabelText('File name', {}, SLOW)).toBeDefined()
    expect(upload()).not.toBeNull()
    expect(within(panel).queryByRole('button', { name: /Make private/ })).toBeNull()
  })

  it('offers everything in both halves to someone who manages both, but sharing only with its own grant', async () => {
    library(as('media.manage', 'media.private.manage', 'customers.view'), '/media?area=private')
    fireEvent.click(await tile('deed.jpg'))
    const panel = await details()
    expect(await within(panel).findByRole('region', { name: 'Shared with' }, SLOW)).toBeDefined()
    expect(upload()).not.toBeNull()
    expect(within(panel).getByRole('button', { name: /Make public/ })).toBeDefined()
    expect(within(panel).queryByRole('button', { name: 'Share with a customer' })).toBeNull()
  })

  it('offers sharing with the grant to share and to see customers', async () => {
    library(as('media.view', 'media.private.view', 'media.share', 'customers.view'), '/media?area=private')
    fireEvent.click(await tile('deed.jpg'))
    expect(await within(await details()).findByRole('button', { name: 'Share with a customer' }, SLOW)).toBeDefined()
  })

  it('does not count a private grant given on one channel', async () => {
    const me = meMock({ superuser: false, permissions: ['media.manage', 'media.private.manage'] })
    const scopes = (me.result as { data: { me: { roles: { scopes: { channelId: string | null; permission: { slug: string } }[] }[] } } }).data.me.roles[0]!.scopes
    scopes.find((s) => s.permission.slug === 'media.private.manage')!.channelId = 'channel-eu'
    library([me, ...reads().slice(1)], '/media?area=private')
    expect(await tile('boot.jpg')).toBeDefined()
    expect(screen.queryByRole('group', { name: 'Which files' })).toBeNull()
  })

  it('holds back a move to the other half while words typed for the file are unsaved', async () => {
    library(as('media.manage', 'media.private.manage'))
    fireEvent.click(await tile('boot.jpg'))
    const panel = await details()
    const cross = await within(panel).findByRole('button', { name: /Make private/ }, SLOW)
    expect(cross).toHaveProperty('disabled', false)

    fireEvent.change(within(panel).getByLabelText('Title'), { target: { value: 'Boot, brown' } })
    await waitFor(() => expect(within(panel).getByRole('button', { name: /Make private/ })).toHaveProperty('disabled', true))
    expect(within(panel).getByText(/Save or undo your other changes/)).toBeDefined()

    fireEvent.click(within(panel).getByRole('button', { name: 'Undo' }))
    await waitFor(() => expect(within(panel).getByRole('button', { name: /Make private/ })).toHaveProperty('disabled', false))
    fireEvent.change(within(panel).getByLabelText('File name'), { target: { value: 'boots.jpg' } })
    expect(within(panel).getByRole('button', { name: /Make private/ })).toHaveProperty('disabled', true)
  })

  it('asks before dropping a share being filled in, as it does for unsaved words', async () => {
    library(as('media.view', 'media.private.view', 'media.share', 'customers.view'), '/media?area=private')
    fireEvent.click(await tile('deed.jpg'))
    fireEvent.click(await within(await details()).findByRole('button', { name: 'Share with a customer' }, SLOW))
    fireEvent.change(screen.getByLabelText('Download limit'), { target: { value: '3' } })
    fireEvent.click(within((await halves())[0]!).getByRole('button', { name: 'Public' }))

    const ask = await screen.findByRole('alertdialog')
    expect(within(ask).getByText("What you typed for it hasn't been saved.")).toBeDefined()
    fireEvent.click(within(ask).getByRole('button', { name: 'Discard' }))
    expect(await tile('boot.jpg')).toBeDefined()
  })
})
