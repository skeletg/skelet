import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import type { MockLink } from '@apollo/client/testing'
import { MemoryRouter } from 'react-router-dom'
import { renderDialog } from '@/test/page-harness'
import { ChildrenSection } from './ChildrenSection'
import { RemoveProductChildDocument } from './Catalog.gql'

// What this pins: removing a configurable's child confirms through the shared ConfirmDialog - a refused remove
// keeps it open with the server's message, a successful one closes it

const child = { __typename: 'Product', id: 'c1', url: '/c1', name: 'Red tee', sku: 'TEE-R', defaultPrice: 1000, defaultCurrencyCode: 'EUR' }
const parent = {
  id: 'p1', sku: 'TEE', defaultPrice: 1000, defaultCurrencyCode: 'EUR', children: [child], configurableOptions: [],
  productType: { id: 'pt1', slug: 'tee', attributes: [] },
}
const remove = { query: RemoveProductChildDocument, variables: { parentId: 'p1', childId: 'c1' } }

async function confirmRemove(mock: MockLink.MockedResponse) {
  renderDialog(<MemoryRouter><ChildrenSection product={parent} canManage /></MemoryRouter>, [mock])
  fireEvent.click(screen.getByRole('button', { name: 'Remove Red tee' }))
  const dialog = await screen.findByRole('alertdialog')
  fireEvent.click(within(dialog).getByRole('button', { name: 'Remove child' }))
  return dialog
}

describe('ChildrenSection remove', () => {
  it('keeps the dialog open and shows a refused remove', async () => {
    const dialog = await confirmRemove({ request: remove, result: { errors: [{ message: 'child has orders' }] } })
    expect(await within(dialog).findByText('child has orders')).toBeDefined()
    expect(screen.getByRole('alertdialog')).toBe(dialog)
  })

  it('closes once the remove goes through', async () => {
    await confirmRemove({ request: remove, result: { data: { removeProductChild: { __typename: 'Product', id: 'p1', children: [] } } } })
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
  })
})
