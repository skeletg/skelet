import { fireEvent, screen, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, renderSection } from '@/test/page-harness'
import { SetProductListingDocument } from '../Catalog.gql'
import { ChannelUrlOverridesEditor } from './ChannelUrlOverridesEditor'

// What this pins: only the stores whose URL changed are written, and an emptied one goes back to
// inheriting (null)

describe('ChannelUrlOverridesEditor', () => {
  it('writes the changed stores only', async () => {
    const write = vi.fn()
    renderSection(
      <ChannelUrlOverridesEditor
        productId="p1"
        listings={[
          { url: 'eu/tee', channel: { id: 'c1', name: 'EU' } },
          { url: null, channel: { id: 'c2', name: 'US' } },
          { url: null, channel: { id: 'c3', name: 'UK' } },
        ]}
        canManage
      />,
      [
        {
          request: {
            query: SetProductListingDocument,
            variables: (vars: Record<string, unknown>) => {
              write(vars)
              return true
            },
          },
          result: { data: { setProductListing: null } },
          maxUsageCount: 3,
        },
      ],
    )
    fireEvent.change(screen.getByLabelText('EU'), { target: { value: '' } })
    fireEvent.change(screen.getByLabelText('US'), { target: { value: 'us/tee' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    const dialog = await screen.findByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply 2 changes/ }))
    await vi.waitFor(() => expect(write).toHaveBeenCalledTimes(2), SLOW)
    expect(write).toHaveBeenCalledWith({ input: expect.objectContaining({ channelId: 'c1', url: null }) })
    expect(write).toHaveBeenCalledWith({ input: expect.objectContaining({ channelId: 'c2', url: 'us/tee' }) })
  })
})
