import type { ReactNode } from 'react'
import { renderHook } from '@testing-library/react'
import { InMemoryCache } from '@apollo/client'
import { MockedProvider } from '@apollo/client/testing/react'
import { describe, expect, it, vi } from 'vitest'
import { SetProductListingDocument } from '../Catalog.gql'
import { useProductListingWrite } from './productListing'

// What this pins: the listing write replaces the store's whole listing, so a URL change still
// carries the price and published flag already on it

describe('useProductListingWrite', () => {
  it('sends the rest of the listing along with the change', async () => {
    const cache = new InMemoryCache()
    cache.restore({
      ROOT_QUERY: { __typename: 'Query', 'product({"id":"p1"})': { __ref: 'Product:p1' } },
      'Product:p1': {
        __typename: 'Product',
        id: 'p1',
        listings: [{ __typename: 'ProductListing', isPublished: true, price: 1200, url: null, channel: { __ref: 'Channel:c1' } }],
      },
      'Channel:c1': { __typename: 'Channel', id: 'c1', name: 'EU' },
    })
    const write = vi.fn()
    const wrapper = ({ children }: { children: ReactNode }) => (
      <MockedProvider
        cache={cache}
        mocks={[
          {
            request: {
              query: SetProductListingDocument,
              variables: (vars: Record<string, unknown>) => {
                write(vars)
                return true
              },
            },
            result: { data: { setProductListing: null } },
          },
        ]}
      >
        {children}
      </MockedProvider>
    )
    const { result } = renderHook(() => useProductListingWrite('p1'), { wrapper })
    await result.current('c1', { url: 'eu/tee' })
    expect(write).toHaveBeenCalledWith({ input: { productId: 'p1', channelId: 'c1', isPublished: true, price: 1200, url: 'eu/tee' } })
  })
})
