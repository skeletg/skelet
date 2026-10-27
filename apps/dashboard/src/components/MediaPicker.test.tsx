import { fireEvent, render, screen } from '@testing-library/react'
import { MockedProvider } from '@apollo/client/testing/react'
import { describe, expect, it, vi } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import { MediaPicker, MediaPickerButton } from './MediaPicker'
import { MediaAssetByIdDocument, SearchMediaAssetsDocument } from '@/features/media/MediaLibrary.gql'
import { MediaPickerAssetDocument } from './MediaPicker.gql'

// What this pins: both pickers browse through the shared media picker - the card hands back the asset's id once
// "Use picture" confirms it, the email builder's button its url on a double click - and the card opens the picker on
// the image it already shows

const asset = {
  __typename: 'MediaAsset', id: 'a1', path: 'promo/sticker.png', directoryPath: 'promo', filename: 'sticker.png',
  url: 'https://cdn/sticker.png', mimeType: 'image/png', mimeGroup: 'IMAGE', sizeBytes: 100, width: 10, height: 10,
  status: 'READY', title: 'Sale sticker', altText: '', caption: '', notes: '', focalX: null, focalY: null,
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
    render(<MemoryRouter><MockedProvider mocks={[search]}><MediaPicker value={null} onChange={onChange} /></MockedProvider></MemoryRouter>)
    fireEvent.click(screen.getByRole('button', { name: /Browse/ }))
    fireEvent.click(await screen.findByRole('option', { name: /Sale sticker/ }))
    fireEvent.click(screen.getByRole('button', { name: 'Use picture' }))
    expect(onChange).toHaveBeenCalledWith('a1')
  })

  it('hands the email builder the picked url', async () => {
    const onPick = vi.fn()
    render(<MemoryRouter><MockedProvider mocks={[search]}><MediaPickerButton onPick={onPick} /></MockedProvider></MemoryRouter>)
    fireEvent.click(screen.getByRole('button', { name: /Library/ }))
    fireEvent.doubleClick(await screen.findByRole('option', { name: /Sale sticker/ }))
    expect(onPick).toHaveBeenCalledWith('https://cdn/sticker.png')
  })

  it('opens on the image the card already shows, and removes it', async () => {
    const onChange = vi.fn()
    const mocks = [
      search,
      { request: { query: MediaPickerAssetDocument, variables: { id: 'a1' } }, result: { data: { mediaAsset: { __typename: 'MediaAsset', id: 'a1', url: asset.url, filename: asset.filename, title: asset.title, altText: '' } } } },
      { request: { query: MediaAssetByIdDocument, variables: { id: 'a1' } }, result: { data: { mediaAsset: asset } } },
    ]
    render(<MemoryRouter><MockedProvider mocks={mocks}><MediaPicker value="a1" onChange={onChange} /></MockedProvider></MemoryRouter>)
    fireEvent.click(await screen.findByRole('button', { name: /Replace/ }))
    const tile = await screen.findByRole('option', { name: /Sale sticker/ })
    expect(tile.getAttribute('aria-selected')).toBe('true')
    expect(screen.getByRole('button', { name: 'Use picture' })).toHaveProperty('disabled', false)
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    fireEvent.click(screen.getByRole('button', { name: /Remove/ }))
    expect(onChange).toHaveBeenCalledExactlyOnceWith(null)
  })
})
