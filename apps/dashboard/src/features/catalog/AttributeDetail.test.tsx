import { fireEvent, screen, within } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { SLOW, captureInput, meMock, renderPage } from '@/test/page-harness'
import { AttributeDetail } from './AttributeDetail'
import { AttributeDocument, UpdateAttributeDocument } from './Catalog.gql'

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
