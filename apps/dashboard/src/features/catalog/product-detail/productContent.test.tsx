import type { ReactNode } from 'react'
import { renderHook } from '@testing-library/react'
import { InMemoryCache } from '@apollo/client'
import { MockedProvider } from '@apollo/client/testing/react'
import { describe, expect, it, vi } from 'vitest'
import { UpsertProductContentDocument } from '../Catalog.gql'
import { useProductContentWrite } from './productContent'

// What this pins: the upsert replaces the locale's whole row, so a write that only changes the name
// still carries the descriptions and SEO fields already on it

describe('useProductContentWrite', () => {
  it('sends the rest of the row along with the change', async () => {
    const cache = new InMemoryCache()
    cache.restore({
      ROOT_QUERY: { __typename: 'Query', 'product({"id":"p1"})': { __ref: 'Product:p1' } },
      'Product:p1': {
        __typename: 'Product',
        id: 'p1',
        name: 'Tee',
        content: [{ __typename: 'ProductContent', locale: 'en', name: 'Tee', shortDescription: { root: 'short' }, descriptionRichText: null, metaTitle: 'Buy tee', metaDescription: null, metaKeywords: null }],
      },
    })
    const write = vi.fn()
    const wrapper = ({ children }: { children: ReactNode }) => (
      <MockedProvider
        cache={cache}
        mocks={[
          {
            request: {
              query: UpsertProductContentDocument,
              variables: (vars: Record<string, unknown>) => {
                write(vars)
                return true
              },
            },
            result: { data: { upsertProductContent: null } },
          },
        ]}
      >
        {children}
      </MockedProvider>
    )
    const { result } = renderHook(() => useProductContentWrite('p1', 'en'), { wrapper })
    await result.current({ name: 'Classic tee' })
    expect(write).toHaveBeenCalledWith({
      productId: 'p1',
      locale: 'en',
      input: { name: 'Classic tee', shortDescription: { root: 'short' }, metaTitle: 'Buy tee' },
    })
  })
})
