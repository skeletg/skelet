import { fireEvent, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, captureInput, meMock, renderPage } from '@/test/page-harness'
import { ChannelsDocument } from '@/features/channels/Channels.gql'
import { CollectionCreate } from './CollectionCreate'
import { CollectionsDocument, CreateCollectionDocument } from './Collections.gql'
import { newCollectionSchema, toNewCollectionInput, emptyNewCollection } from './schemas/newCollection'

describe('new collection schema', () => {
  it('uses the default locale when none was picked, and skips an empty description', () => {
    const values = { ...emptyNewCollection, name: 'Summer', slug: 'summer' }
    expect(newCollectionSchema('en').safeParse(values).success).toBe(true)
    expect(newCollectionSchema('').safeParse(values).error?.issues[0].path).toEqual(['locale'])
    expect(toNewCollectionInput({ ...values, description: { root: {} }, descriptionText: ' ' }, 'en')).toMatchObject({
      locale: 'en',
      description: undefined,
    })
  })
})

describe('CollectionCreate', () => {
  it('rejects a bad slug, then creates and opens the collection', async () => {
    const onCreate = vi.fn()
    renderPage({
      path: '/collections/new',
      routes: { '/collections/new': <CollectionCreate />, '/collections/:id': <p>collection page</p> },
      mocks: [
        meMock(),
        { request: { query: ChannelsDocument }, result: { data: { channels: [] } } },
        {
          request: { query: CreateCollectionDocument, variables: captureInput(onCreate) },
          result: { data: { createCollection: { __typename: 'Collection', id: 'col1' } } },
        },
        { request: { query: CollectionsDocument, variables: () => true }, result: { data: { collections: [] } }, maxUsageCount: 3 },
      ],
    })
    fireEvent.change(await screen.findByLabelText('Name'), { target: { value: 'Summer arrivals' } })
    fireEvent.change(screen.getByLabelText('Slug'), { target: { value: 'Summer Arrivals' } })
    fireEvent.click(screen.getByRole('button', { name: 'Create collection' }))
    expect(await screen.findByText('Lowercase letters, numbers, underscores and hyphens only')).toBeDefined()

    fireEvent.change(screen.getByLabelText('Slug'), { target: { value: 'summer-arrivals' } })
    fireEvent.change(screen.getByLabelText('Locale'), { target: { value: 'en' } })
    fireEvent.click(screen.getByRole('button', { name: 'Create collection' }))
    expect(await screen.findByText('collection page', {}, SLOW)).toBeDefined()
    expect(onCreate).toHaveBeenCalledWith(
      expect.objectContaining({ slug: 'summer-arrivals', name: 'Summer arrivals', locale: 'en', isPublished: false }),
    )
  })
})
