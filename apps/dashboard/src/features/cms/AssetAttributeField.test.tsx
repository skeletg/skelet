import { render, screen } from '@testing-library/react'
import { MockedProvider } from '@apollo/client/testing/react'
import { InMemoryCache } from '@apollo/client'
import { describe, expect, it, vi } from 'vitest'
import { MediaAssetByIdDocument } from '@/features/media/MediaLibrary.gql'
import { AssetsField } from './AssetAttributeField'

// What this pins: a gallery field fetches each asset's thumbnail once - remounting it (opening a block again, switching
// tabs) used to refetch every image under the app's network-only default

const asset = {
  __typename: 'MediaAsset',
  id: 'm1',
  path: 'a/shoe.jpg',
  directoryPath: 'a',
  filename: 'shoe.jpg',
  url: '/files/a/shoe.jpg',
  mimeType: 'image/jpeg',
  mimeGroup: 'image',
  sizeBytes: 1,
  width: 1,
  height: 1,
  status: 'tracked',
  title: 'Shoe',
  altText: 'A shoe',
  caption: '',
  notes: '',
  focalX: null,
  focalY: null,
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
}

describe('AssetsField thumbnails', () => {
  it('fetches an asset once across remounts', async () => {
    const asked = vi.fn()
    const cache = new InMemoryCache()
    const mocks = [
      {
        request: {
          query: MediaAssetByIdDocument,
          variables: () => {
            asked()
            return true
          },
        },
        result: { data: { mediaAsset: asset } },
        maxUsageCount: 5,
      },
    ]
    const field = () => (
      <MockedProvider mocks={mocks} cache={cache} defaultOptions={{ watchQuery: { fetchPolicy: 'network-only' } }}>
        <AssetsField value={['m1']} onChange={() => {}} />
      </MockedProvider>
    )
    const first = render(field())
    expect(await screen.findByAltText('A shoe')).toBeTruthy()
    first.unmount()
    render(field())
    expect(await screen.findByAltText('A shoe')).toBeTruthy()
    expect(asked).toHaveBeenCalledTimes(1)
  })
})
