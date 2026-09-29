import { fireEvent, screen, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'sonner'
import { SLOW, captureInput, renderSection } from '@/test/page-harness'
import { ChannelsDocument } from '@/features/channels/Channels.gql'
import { ClearProductListingDocument, SetProductListingDocument } from './Catalog.gql'
import { ProductListingsEditor } from './ProductListingsEditor'

vi.mock('sonner', () => ({ toast: { warning: vi.fn() } }))

// What these pin: a staged clear removes the listing, a publish writes only that store, a store
// set to its own price with none typed is refused rather than saved as zero, and a wrapper with nothing
// to sell can still be published but warns it will show out of stock

const channel = (id: string, name: string) => ({
  __typename: 'Channel' as const,
  id,
  slug: name.toLowerCase(),
  name,
  defaultLocale: 'en',
  locales: [],
  isActive: true,
  isDefault: false,
  currencyCode: 'EUR',
  publicUrl: null,
  metadata: [],
  createdAt: '2026-01-01T00:00:00Z',
})

const product = {
  id: 'p1',
  listings: [{ isPublished: true, price: 1200, effectivePrice: 1200, effectiveCurrency: 'EUR', channel: { id: 'c1' } }],
}

function renderListings(p: Parameters<typeof ProductListingsEditor>[0]['product'] = product) {
  const writes = { set: vi.fn(), clear: vi.fn() }
  renderSection(<ProductListingsEditor product={p} canManage />, [
    { request: { query: ChannelsDocument }, result: { data: { channels: [channel('c1', 'Wheels'), channel('c2', 'Tyres')] } }, maxUsageCount: 3 },
    { request: { query: SetProductListingDocument, variables: captureInput(writes.set) }, result: { data: { setProductListing: null } }, maxUsageCount: 3 },
    {
      request: { query: ClearProductListingDocument, variables: captureInput(writes.clear) },
      result: { data: { clearProductListing: true } },
      maxUsageCount: 3,
    },
  ])
  return writes
}

async function apply(changes: RegExp) {
  fireEvent.click(await screen.findByRole('button', { name: 'Save changes' }))
  const dialog = await screen.findByRole('alertdialog')
  fireEvent.click(within(dialog).getByRole('button', { name: changes }))
  return dialog
}

describe('ProductListingsEditor', () => {
  beforeEach(() => vi.mocked(toast.warning).mockClear())

  it('clears one store and publishes the other', async () => {
    const writes = renderListings()
    fireEvent.click(await screen.findByRole('button', { name: 'Clear Wheels' }, SLOW))
    fireEvent.click(screen.getByRole('switch', { name: 'Tyres published' }))
    await apply(/Apply 2 changes/)
    await vi.waitFor(() => expect(writes.set).toHaveBeenCalled(), SLOW)
    expect(writes.clear).toHaveBeenCalledWith({ productId: 'p1', channelId: 'c1' })
    expect(writes.set).toHaveBeenCalledWith(expect.objectContaining({ channelId: 'c2', isPublished: true, price: null }))
  })

  it('refuses an own price left blank', async () => {
    const writes = renderListings()
    await screen.findByText('Tyres', undefined, SLOW)
    fireEvent.click(screen.getAllByRole('switch', { name: 'Use default' })[1])
    const dialog = await apply(/Apply/)
    expect(await within(dialog).findByText(/Zero or more, or use the default/)).toBeDefined()
    expect(writes.set).not.toHaveBeenCalled()
  })

  it('lets an empty configurable be published, warning it will show out of stock', async () => {
    const writes = renderListings({ ...product, kind: 'CONFIGURABLE', children: [] })
    expect(await screen.findByText(/has no variants, so it will show as out of stock/, undefined, SLOW)).toBeDefined()
    const tyres = screen.getByRole('switch', { name: 'Tyres published' })
    expect(tyres).toHaveProperty('disabled', false)
    fireEvent.click(tyres)
    expect(toast.warning).toHaveBeenCalledWith(expect.stringMatching(/^Publishing on Tyres: .*out of stock/))
    await apply(/Apply/)
    await vi.waitFor(() => expect(writes.set).toHaveBeenCalledWith(expect.objectContaining({ channelId: 'c2', isPublished: true })), SLOW)
  })

  it('says nothing once the configurable has a variant', async () => {
    renderListings({ ...product, kind: 'CONFIGURABLE', children: [{ id: 'v1' }] })
    fireEvent.click(await screen.findByRole('switch', { name: 'Tyres published' }, SLOW))
    expect(screen.queryByText(/Nothing to sell yet/)).toBeNull()
    expect(toast.warning).not.toHaveBeenCalled()
  })
})
