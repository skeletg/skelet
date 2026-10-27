import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import type { MockLink } from '@apollo/client/testing'
import { renderDialog } from '@/test/page-harness'
import { ProductDownloadsSection } from './ProductDownloadsSection'
import { RemoveProductDownloadDocument } from './Catalog.gql'

// What this pins: removing a download confirms through the shared ConfirmDialog - a refused remove keeps it open
// with the server's message (it used to land in the section's banner behind the dialog), a successful one closes it

const manual = { __typename: 'ProductDownload' as const, id: 'd1', title: 'Manual', url: 'https://example.com/m.pdf', maxDownloads: null, sortOrder: 0, createdAt: '2026-10-01T00:00:00Z', updatedAt: '2026-10-01T00:00:00Z' }
const remove = { query: RemoveProductDownloadDocument, variables: { id: 'd1' } }

async function confirmRemove(mock: MockLink.MockedResponse) {
  renderDialog(<ProductDownloadsSection product={{ id: 'p1', downloads: [manual] }} canManage />, [mock])
  const row = screen.getByText('Manual').parentElement!.parentElement!.parentElement as HTMLElement
  fireEvent.click(within(row).getAllByRole('button').at(-1)!)
  const dialog = await screen.findByRole('alertdialog')
  fireEvent.click(within(dialog).getByRole('button', { name: 'Remove download' }))
  return dialog
}

describe('ProductDownloadsSection remove', () => {
  it('keeps the dialog open and shows a refused remove', async () => {
    const dialog = await confirmRemove({ request: remove, result: { errors: [{ message: 'download is granted' }] } })
    expect(await within(dialog).findByText('download is granted')).toBeDefined()
    expect(screen.getByRole('alertdialog')).toBe(dialog)
  })

  it('closes once the remove goes through', async () => {
    await confirmRemove({ request: remove, result: { data: { removeProductDownload: { __typename: 'Product', id: 'p1', downloads: [] } } } })
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
  })
})
