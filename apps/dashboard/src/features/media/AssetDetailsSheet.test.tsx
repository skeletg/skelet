import { fireEvent, screen, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, renderPage } from '@/test/page-harness'
import { AssetDetailsSheet } from './MediaLibrary'

// What these pin: closing the sheet with edited metadata asks before throwing the edits away - it used to drop
// them silently - and closing without edits still closes straight away

const asset = {
  __typename: 'MediaAsset' as const,
  id: 'm1',
  path: 'products/shoe.jpg',
  directoryPath: 'products',
  filename: 'shoe.jpg',
  url: '/files/products/shoe.jpg',
  mimeType: 'image/jpeg',
  mimeGroup: 'image',
  sizeBytes: 1024,
  width: 800,
  height: 600,
  status: 'tracked',
  title: 'Shoe',
  altText: '',
  caption: '',
  notes: '',
  focalX: null,
  focalY: null,
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
}

function renderSheet() {
  const onOpenChange = vi.fn()
  renderPage({
    path: '/media',
    adminId: 'admin-1',
    routes: {
      '/media': (
        <AssetDetailsSheet
          asset={asset as never}
          canManage
          currentPath="products"
          onOpenChange={onOpenChange}
          onChanged={() => {}}
          onDeleted={() => {}}
          onError={() => {}}
        />
      ),
    },
    mocks: [],
  })
  return onOpenChange
}

describe('AssetDetailsSheet', () => {
  it('asks before discarding edited metadata', async () => {
    const onOpenChange = renderSheet()
    fireEvent.change(await screen.findByLabelText('Alt text', {}, SLOW), { target: { value: 'A red running shoe' } })
    fireEvent.keyDown(document.activeElement ?? document.body, { key: 'Escape' })
    const confirm = await screen.findByRole('alertdialog')
    expect(onOpenChange).not.toHaveBeenCalled()

    fireEvent.click(within(confirm).getByRole('button', { name: 'Discard' }))
    await vi.waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false), SLOW)
  })

  it('closes straight away when nothing was edited', async () => {
    const onOpenChange = renderSheet()
    await screen.findByLabelText('Alt text', {}, SLOW)
    fireEvent.keyDown(document.activeElement ?? document.body, { key: 'Escape' })
    await vi.waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false), SLOW)
    expect(screen.queryByRole('alertdialog')).toBeNull()
  })
})
