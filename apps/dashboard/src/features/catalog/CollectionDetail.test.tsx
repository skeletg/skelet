import { fireEvent, screen, within } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { SLOW, meMock, renderPage } from '@/test/page-harness'
import { CollectionDetail } from './CollectionDetail'
import { CollectionDocument, UpdateCollectionDocument, UpsertCollectionContentDocument } from './Collections.gql'

// What these pin: the translation and the record's own fields are one review, a duplicate metadata
// key and a nameless translation are refused, and each write goes only when its part changed

const collection = {
  __typename: 'Collection' as const,
  id: 'col1',
  slug: 'summer',
  isPublished: false,
  seoNoindex: false,
  name: 'Summer',
  description: null,
  metaTitle: null,
  metaDescription: null,
  metaKeywords: null,
  contentLocale: 'en',
  content: [
    {
      __typename: 'CollectionContent' as const,
      locale: 'en',
      name: 'Summer',
      description: null,
      metaTitle: null,
      metaDescription: null,
      metaKeywords: null,
      updatedAt: '2026-01-01T00:00:00Z',
    },
  ],
  coverImage: null,
  products: [],
  metadata: [{ __typename: 'MetadataItem' as const, key: 'season', value: 'ss26' }],
  privateMetadata: [],
  facetOverrides: [],
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
}

const recorder = (fn: (v: Record<string, unknown>) => void) => (vars: Record<string, unknown>) => {
  fn(vars)
  return true
}

function renderCollection() {
  const writes = { update: vi.fn(), content: vi.fn() }
  renderPage({
    path: '/collections/col1',
    adminId: 'admin-1',
    routes: { '/collections/:id': <CollectionDetail /> },
    mocks: [
      meMock(),
      { request: { query: CollectionDocument, variables: { id: 'col1' } }, result: { data: { collection } }, maxUsageCount: 5 },
      {
        request: { query: UpdateCollectionDocument, variables: recorder(writes.update) },
        result: { data: { updateCollection: { __typename: 'Collection', id: 'col1', slug: 'summer' } } },
      },
      {
        request: { query: UpsertCollectionContentDocument, variables: recorder(writes.content) },
        result: { data: { upsertCollectionContent: { __typename: 'Collection', id: 'col1' } } },
      },
    ],
  })
  return writes
}

afterEach(() => localStorage.clear())

describe('CollectionDetail', () => {
  it('refuses a duplicate metadata key, then writes the record and the translation', async () => {
    const writes = renderCollection()
    const save = await screen.findByRole('button', { name: 'Save changes' }, SLOW)
    fireEvent.click(screen.getByRole('switch', { name: 'Published' }))
    fireEvent.change(screen.getByLabelText('Meta title'), { target: { value: 'Summer edit' } })
    fireEvent.click(screen.getAllByRole('button', { name: 'Add row' })[0])
    fireEvent.change(screen.getByLabelText('Metadata 2 key'), { target: { value: 'season' } })
    fireEvent.click(save)
    let dialog = await screen.findByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply/ }))
    expect(await within(dialog).findByText(/Duplicate metadata key/)).toBeDefined()
    expect(writes.update).not.toHaveBeenCalled()

    fireEvent.click(within(dialog).getByRole('button', { name: 'Cancel' }))
    fireEvent.click(screen.getByRole('button', { name: 'Remove metadata 2' }))
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    dialog = await screen.findByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply 2 changes/ }))
    await vi.waitFor(() => expect(writes.content).toHaveBeenCalled(), SLOW)
    expect(writes.update).toHaveBeenCalledWith({ input: { id: 'col1', slug: 'summer', isPublished: true, seoNoindex: false } })
    expect(writes.content).toHaveBeenCalledWith(
      expect.objectContaining({ collectionId: 'col1', locale: 'en', input: expect.objectContaining({ name: 'Summer', metaTitle: 'Summer edit' }) }),
    )
  })

  it('refuses a translation edit without a name', async () => {
    const writes = renderCollection()
    const save = await screen.findByRole('button', { name: 'Save changes' }, SLOW)
    fireEvent.change(screen.getByLabelText('Name'), { target: { value: '' } })
    fireEvent.click(save)
    const dialog = await screen.findByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply/ }))
    expect(await within(dialog).findByText(/A translation needs a name/)).toBeDefined()
    expect(writes.content).not.toHaveBeenCalled()
  })
})
