import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { SLOW, captureInput, meMock, renderPage } from '@/test/page-harness'
import { AttributeDetail } from './AttributeDetail'
import type { MockLink } from '@apollo/client/testing'
import { AddAttributeValueDocument, AttributeDocument, AttributesDocument, DeleteAttributeDocument, SetAttributeIndexableDocument, UpdateAttributeDocument } from './Catalog.gql'
import { ConfigSectionsDocument } from '@/features/settings/Settings.gql'

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
  embedded: true,
  similarMatch: false,
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

// What the page reads around the attribute - the search tuning's settings, and the list a save refetches
const around = [
  { request: { query: ConfigSectionsDocument, variables: { scope: { kind: 'DEFAULT' } } }, result: { data: { configSections: [] } }, maxUsageCount: 10 },
  { request: { query: AttributesDocument }, result: { data: { attributes: [] } }, maxUsageCount: 10 },
]

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
        ...around,
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

  it('keeps an attribute out of search by meaning, through the save confirm', async () => {
    const onUpdate = vi.fn()
    renderPage({
      path: '/catalog/attributes/at1',
      adminId: 'admin-1',
      routes: { '/catalog/attributes/:id': <AttributeDetail /> },
      mocks: [
        meMock(),
        ...around,
        { request: { query: AttributeDocument, variables: { id: 'at1' } }, result: { data: { attribute } }, maxUsageCount: 5 },
        {
          request: { query: UpdateAttributeDocument, variables: captureInput(onUpdate) },
          result: { data: { updateAttribute: { ...attribute, embedded: false } } },
        },
      ],
    })

    await screen.findByRole('button', { name: 'Save changes' }, SLOW)
    fireEvent.click(screen.getByRole('switch', { name: 'Search by meaning' }))
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    const dialog = await screen.findByRole('alertdialog')
    expect(within(dialog).getByText('Search by meaning')).toBeDefined()
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply 1 change/ }))
    await vi.waitFor(() => expect(onUpdate).toHaveBeenCalled(), SLOW)
    expect(onUpdate).toHaveBeenCalledWith(expect.objectContaining({ id: 'at1', embedded: false }))
  })

  it('makes similar products keep to this attribute, through the save confirm', async () => {
    const onUpdate = vi.fn()
    renderPage({
      path: '/catalog/attributes/at1',
      adminId: 'admin-1',
      routes: { '/catalog/attributes/:id': <AttributeDetail /> },
      mocks: [
        meMock(),
        ...around,
        { request: { query: AttributeDocument, variables: { id: 'at1' } }, result: { data: { attribute } }, maxUsageCount: 5 },
        {
          request: { query: UpdateAttributeDocument, variables: captureInput(onUpdate) },
          result: { data: { updateAttribute: { ...attribute, similarMatch: true } } },
        },
      ],
    })

    await screen.findByRole('button', { name: 'Save changes' }, SLOW)
    fireEvent.click(screen.getByRole('switch', { name: 'Must match for similar products' }))
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    const dialog = await screen.findByRole('alertdialog')
    expect(within(dialog).getByText('Must match for similar products')).toBeDefined()
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply 1 change/ }))
    await vi.waitFor(() => expect(onUpdate).toHaveBeenCalled(), SLOW)
    expect(onUpdate).toHaveBeenCalledWith(expect.objectContaining({ id: 'at1', similarMatch: true }))
  })

  it('adds a value that stands for every other one, through the save confirm', async () => {
    const gender = { ...attribute, slug: 'gender', name: 'Gender', inputType: 'DROPDOWN' as const, unit: '', similarMatch: true }
    const added = vi.fn()
    renderPage({
      path: '/catalog/attributes/at1',
      adminId: 'admin-1',
      routes: { '/catalog/attributes/:id': <AttributeDetail /> },
      mocks: [
        meMock(),
        ...around,
        { request: { query: AttributeDocument, variables: { id: 'at1' } }, result: { data: { attribute: gender } }, maxUsageCount: 5 },
        {
          request: {
            query: AddAttributeValueDocument,
            variables: (v: Record<string, unknown>) => {
              added(v)
              return true
            },
          },
          result: { data: { addAttributeValue: { __typename: 'AttributeValue', id: 'v1', slug: 'unisex', name: 'Unisex', value: '', sortOrder: 0, isIndexable: false, matchesAny: true } } },
        },
      ],
    })

    fireEvent.click(await screen.findByRole('button', { name: /Add value/ }, SLOW))
    fireEvent.change(screen.getByPlaceholderText('Red'), { target: { value: 'Unisex' } })
    fireEvent.click(screen.getByRole('switch', { name: /Matches every value/ }))
    fireEvent.click(screen.getByRole('button', { name: 'Add' }))
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    const dialog = await screen.findByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply/ }))
    await vi.waitFor(() => expect(added).toHaveBeenCalled(), SLOW)
    expect(added).toHaveBeenCalledWith(expect.objectContaining({ attributeId: 'at1', input: expect.objectContaining({ name: 'Unisex', slug: 'unisex', matchesAny: true }) }))
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
      mocks: [meMock(), ...around, { request: { query: AttributeDocument, variables: { id: 'at1' } }, result: { data: { attribute } }, maxUsageCount: 5 }, mock],
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
