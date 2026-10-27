import { act, fireEvent, render, screen } from '@testing-library/react'
import { MockedProvider } from '@apollo/client/testing/react'
import { InMemoryCache } from '@apollo/client'
import { describe, expect, it, vi } from 'vitest'
import type { MediaPickerProps } from '@/features/media/picker/MediaPickerDialog'
import { MediaAssetByIdDocument } from '@/features/media/MediaLibrary.gql'
import { AssetField, AssetsField } from './AssetAttributeField'

// What this pins: a gallery field fetches each asset's thumbnail once - remounting it (opening a block again, switching
// tabs) used to refetch every image under the app's network-only default. And both fields drive the picker the right
// way - the single one opens on its current image, the gallery picks several and adds them once each, in order

// The picker has its own tests - here it only records what the field hands it
const picker = vi.hoisted(() => ({ props: null as MediaPickerProps | null }))
vi.mock('@/features/media/MediaAssetPicker', () => ({
  MediaAssetPicker: (p: MediaPickerProps) => {
    picker.props = p
    return null
  },
}))

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

const any = { request: { query: MediaAssetByIdDocument, variables: () => true }, result: { data: { mediaAsset: asset } }, maxUsageCount: 10 }
const picked = (id: string) => ({ ...asset, id })

describe('AssetField', () => {
  it('opens the picker on its current image and keeps what is picked', () => {
    const onChange = vi.fn()
    render(<MockedProvider mocks={[any]}><AssetField value="m1" onChange={onChange} /></MockedProvider>)
    fireEvent.click(screen.getByRole('button', { name: 'Replace' }))
    expect(picker.props).toMatchObject({ open: true, selectedIds: ['m1'], uploadPath: 'cms/pages' })
    expect(picker.props?.multiple).toBeFalsy()
    act(() => picker.props?.onSelect?.(picked('m2') as never))
    expect(onChange).toHaveBeenCalledWith('m2')
  })

  it('opens with nothing picked when empty, and removes on request', () => {
    const onChange = vi.fn()
    const { rerender } = render(<MockedProvider mocks={[any]}><AssetField value="" onChange={onChange} /></MockedProvider>)
    fireEvent.click(screen.getByRole('button', { name: 'Choose from library' }))
    expect(picker.props?.selectedIds).toEqual([])
    rerender(<MockedProvider mocks={[any]}><AssetField value="m1" onChange={onChange} /></MockedProvider>)
    fireEvent.click(screen.getByRole('button', { name: 'Remove' }))
    expect(onChange).toHaveBeenCalledWith(undefined)
  })
})

describe('AssetsField picking', () => {
  it('picks several and adds each one once, after what is there', () => {
    const onChange = vi.fn()
    render(<MockedProvider mocks={[any]}><AssetsField value={['m1']} onChange={onChange} /></MockedProvider>)
    fireEvent.click(screen.getByRole('button', { name: 'Add' }))
    expect(picker.props).toMatchObject({ open: true, multiple: true, uploadPath: 'cms/pages' })
    act(() => picker.props?.onSelectMany?.([picked('m3'), picked('m1'), picked('m2')] as never))
    expect(onChange).toHaveBeenCalledWith(['m1', 'm3', 'm2'])
  })

  it('removes the image whose cross was clicked', () => {
    const onChange = vi.fn()
    render(<MockedProvider mocks={[any]}><AssetsField value={['m1', 'm2', 'm1']} onChange={onChange} /></MockedProvider>)
    fireEvent.click(screen.getAllByRole('button', { name: 'Remove image' })[2])
    expect(onChange).toHaveBeenCalledWith(['m1', 'm2'])
  })
})
