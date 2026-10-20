import { fireEvent, render, screen } from '@testing-library/react'
import { MockedProvider } from '@apollo/client/testing/react'
import { describe, expect, it, vi } from 'vitest'
import { MediaPicker, MediaPickerButton } from './MediaPicker'
import { SearchMediaAssetsDocument } from '@/features/media/MediaLibrary.gql'

// What this pins: both pickers browse through the shared MediaAssetPicker - the card hands back the asset's id, the
// email builder's button its url

const asset = {
  __typename: 'MediaAsset', id: 'a1', path: 'promo/sticker.png', directoryPath: 'promo', filename: 'sticker.png',
  url: 'https://cdn/sticker.png', mimeType: 'image/png', mimeGroup: 'IMAGE', sizeBytes: 100, width: 10, height: 10,
  status: 'ACTIVE', title: 'Sale sticker', altText: '', caption: '', notes: '', focalX: null, focalY: null,
  createdAt: '2026-10-01T00:00:00Z', updatedAt: '2026-10-01T00:00:00Z',
}
const search = {
  request: { query: SearchMediaAssetsDocument, variables: () => true },
  result: { data: { searchMediaAssets: { __typename: 'MediaAssetSearchResult', totalCount: 1, items: [asset] } } },
  maxUsageCount: 5,
}

describe('MediaPicker', () => {
  it('browses the library and hands back the picked asset id', async () => {
    const onChange = vi.fn()
    render(<MockedProvider mocks={[search]}><MediaPicker value={null} onChange={onChange} /></MockedProvider>)
    fireEvent.click(screen.getByRole('button', { name: /Browse/ }))
    fireEvent.click(await screen.findByRole('button', { name: /Sale sticker/ }))
    expect(onChange).toHaveBeenCalledWith('a1')
  })

  it('hands the email builder the picked url', async () => {
    const onPick = vi.fn()
    render(<MockedProvider mocks={[search]}><MediaPickerButton onPick={onPick} /></MockedProvider>)
    fireEvent.click(screen.getByRole('button', { name: /Library/ }))
    fireEvent.click(await screen.findByRole('button', { name: /Sale sticker/ }))
    expect(onPick).toHaveBeenCalledWith('https://cdn/sticker.png')
  })
})
