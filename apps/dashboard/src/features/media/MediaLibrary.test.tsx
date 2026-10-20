import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { SLOW, meMock, renderPage } from '@/test/page-harness'
import { MediaLibrary } from './MediaLibrary'
import { AllMediaDirectoriesDocument, MediaDirectoryDocument } from './MediaLibrary.gql'

// What this pins: the folder tree works from the keyboard - it was mouse-only rows. One tab stop, on the open folder,
// and Enter opens the focused one

const dirs = [
  { __typename: 'MediaDirectoryChild', path: 'shoes', name: 'shoes' },
  { __typename: 'MediaDirectoryChild', path: 'shoes/red', name: 'red' },
  { __typename: 'MediaDirectoryChild', path: 'hats', name: 'hats' },
]
const emptyDir = (path: string) => ({
  request: { query: MediaDirectoryDocument, variables: { path } },
  result: { data: { mediaDirectory: { __typename: 'MediaDirectory', path, name: path, directories: [], assets: [], untrackedFiles: [] } } },
  maxUsageCount: 5,
})

describe('MediaLibrary folder tree', () => {
  it('tabs to the open folder and opens another with Enter', async () => {
    renderPage({
      path: '/media?path=shoes',
      adminId: 'admin-1',
      routes: { '/media': <MediaLibrary /> },
      mocks: [
        meMock(),
        { request: { query: AllMediaDirectoriesDocument }, result: { data: { allMediaDirectories: dirs } }, maxUsageCount: 5 },
        emptyDir('shoes'),
        emptyDir('hats'),
      ],
    })
    const tree = await screen.findByRole('tree', { name: 'Folders' })
    const shoes = await within(tree).findByRole('treeitem', { name: /shoes/ }, SLOW)
    const tabbable = within(tree).getAllByRole('treeitem').filter((r) => r.tabIndex === 0)
    expect(tabbable).toEqual([shoes])
    expect(shoes.getAttribute('aria-expanded')).toBe('true') // the open folder's parent row shows its children

    fireEvent.keyDown(within(tree).getByRole('treeitem', { name: /hats/ }), { key: 'Enter' })
    await waitFor(() => expect(within(tree).getByRole('treeitem', { name: /hats/ }).getAttribute('aria-selected')).toBe('true'))
  })
})
