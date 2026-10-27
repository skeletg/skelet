import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { SLOW, captureInput, meMock, renderPage } from '@/test/page-harness'
import { AttributeDetail } from './AttributeDetail'
import type { MockLink } from '@apollo/client/testing'
import { AttributeDocument, DeleteAttributeDocument, SetAttributeIndexableDocument, UpdateAttributeDocument } from './Catalog.gql'

// What these pin: "required" can't be saved without a usable default (the server would refuse it),
// and the default goes over decoded for the input type, only when it changed

const attribute = {
  __typename: 'Attribute' as const,
  id: 'at1',
  slug: 'weight',
  name: 'Weight',
  inputType: 'NUMERIC' as const,
  relationshipTarget: null,
  unit: 'kg',
  valueRequired: false,
  filterable: false,
  searchable: false,
  isPublic: true,
  facetCollapsed: false,
  isIndexable: false,
  h1Position: 'BEFORE' as const,
  synapseTrack: false,
  autoSegment: false,
  sortOrder: 0,
  defaultValue: null,
  descriptions: [],
  values: [],
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
}

afterEach(() => localStorage.clear())

describe('AttributeDetail', () => {
  it('refuses required without a default, then saves the numeric default', async () => {
    const onUpdate = vi.fn()
    renderPage({
      path: '/catalog/attributes/at1',
      adminId: 'admin-1',
      routes: { '/catalog/attributes/:id': <AttributeDetail /> },
      mocks: [
        meMock(),
        { request: { query: AttributeDocument, variables: { id: 'at1' } }, result: { data: { attribute } }, maxUsageCount: 5 },
        {
          request: { query: UpdateAttributeDocument, variables: captureInput(onUpdate) },
          result: { data: { updateAttribute: { ...attribute, valueRequired: true, defaultValue: 5 } } },
        },
      ],
    })

    const save = await screen.findByRole('button', { name: 'Save changes' }, SLOW)
    fireEvent.click(screen.getByRole('switch', { name: 'Value required' }))
    fireEvent.click(save)
    let dialog = await screen.findByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply/ }))
    expect(await within(dialog).findByText(/must have a default value/)).toBeDefined()

    fireEvent.click(within(dialog).getByRole('button', { name: 'Cancel' }))
    fireEvent.click(screen.getByRole('switch', { name: 'Default value' }))
    fireEvent.change(screen.getByLabelText('Default value', { selector: 'input[type=number]' }), { target: { value: '5' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    dialog = await screen.findByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply 2 changes/ }))
    await vi.waitFor(() => expect(onUpdate).toHaveBeenCalled(), SLOW)
    expect(onUpdate).toHaveBeenCalledWith(expect.objectContaining({ id: 'at1', valueRequired: true, defaultValue: { value: 5 } }))
  })
})

// The delete and the indexable cascade confirm through the shared ConfirmDialog: a refusal keeps the dialog open
// with the server's message, and a successful delete goes back to the list
describe('AttributeDetail confirms', () => {
  function renderAttribute(mock: MockLink.MockedResponse) {
    renderPage({
      path: '/catalog/attributes/at1',
      adminId: 'admin-1',
      routes: { '/catalog/attributes/:id': <AttributeDetail />, '/catalog/attributes': <p>Attributes list</p> },
      mocks: [meMock(), { request: { query: AttributeDocument, variables: { id: 'at1' } }, result: { data: { attribute } }, maxUsageCount: 5 }, mock],
    })
  }

  async function confirmDelete(mock: MockLink.MockedResponse) {
    renderAttribute(mock)
    await screen.findByRole('button', { name: 'Save changes' }, SLOW)
    fireEvent.click(await screen.findByRole('button', { name: 'Delete' }, SLOW))
    const dialog = await screen.findByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: 'Delete attribute' }))
    return dialog
  }

  it('keeps the delete open and shows a refused delete', async () => {
    const dialog = await confirmDelete({ request: { query: DeleteAttributeDocument, variables: { id: 'at1' } }, result: { errors: [{ message: 'attribute in use' }] } })
    expect(await within(dialog).findByText('attribute in use')).toBeDefined()
    expect(screen.getByRole('alertdialog')).toBe(dialog)
  })

  it('goes back to the list once the delete goes through', async () => {
    await confirmDelete({ request: { query: DeleteAttributeDocument, variables: { id: 'at1' } }, result: { data: { deleteAttribute: true } } })
    expect(await screen.findByText('Attributes list', {}, SLOW)).toBeDefined()
  })

  it('keeps the cascade open on a refusal', async () => {
    renderAttribute({ request: { query: SetAttributeIndexableDocument, variables: { attributeId: 'at1', isIndexable: true } }, result: { errors: [{ message: 'too many values' }] } })
    await screen.findByRole('button', { name: 'Save changes' }, SLOW)
    fireEvent.click(screen.getByRole('switch', { name: /Indexable/ }))
    const dialog = await screen.findByRole('alertdialog')
    expect(within(dialog).getByText('Mark this entire facet indexable?')).toBeDefined()
    fireEvent.click(within(dialog).getByRole('button', { name: 'Yes, mark indexable' }))
    expect(await within(dialog).findByText('too many values')).toBeDefined()
    fireEvent.click(within(dialog).getByRole('button', { name: 'Cancel' }))
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
  })
})
