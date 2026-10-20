import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import type { MockLink } from '@apollo/client/testing'
import { renderDialog } from '@/test/page-harness'
import { ProductMediaPanel } from './ProductMediaPanel'
import { DeleteProductMediaDocument } from '../Catalog.gql'

// What this pins: removing an image confirms through the shared ConfirmDialog - a refused remove keeps it open with
// the server's message (it used to land in the panel's banner behind the dialog), a successful one closes it

const image = { id: 'm1', url: 'https://cdn.example.com/a.jpg', altText: 'Front', status: 'READY' as const, sortOrder: 0, width: 800, height: 600, mimeType: 'image/jpeg', sizeBytes: 1000 }
const remove = { query: DeleteProductMediaDocument, variables: { id: 'm1', deleteFromStorage: true } }

async function confirmRemove(mock: MockLink.MockedResponse) {
  renderDialog(
    <ProductMediaPanel productId="p1" media={[image]} roles={{ MAIN: null, OG_IMAGE: null, THUMBNAIL: null }} canManage productVariables={{ id: 'p1' }} />,
    [mock],
  )
  fireEvent.pointerDown(screen.getByRole('button', { name: 'More' }), { button: 0, ctrlKey: false })
  fireEvent.click(await screen.findByRole('menuitem', { name: 'Remove and delete file' }))
  const dialog = await screen.findByRole('alertdialog')
  expect(within(dialog).getByText('Delete image?')).toBeDefined()
  fireEvent.click(within(dialog).getByRole('button', { name: 'Delete permanently' }))
  return dialog
}

describe('ProductMediaPanel remove', () => {
  it('keeps the dialog open and shows a refused remove', async () => {
    const dialog = await confirmRemove({ request: remove, result: { errors: [{ message: 'image is the OG image' }] } })
    expect(await within(dialog).findByText('image is the OG image')).toBeDefined()
    expect(screen.getByRole('alertdialog')).toBe(dialog)
  })

  it('closes once the remove goes through', async () => {
    await confirmRemove({ request: remove, result: { data: { deleteProductMedia: true } } })
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
  })
})
