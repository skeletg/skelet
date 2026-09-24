import { fireEvent, screen, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { MockLink } from '@apollo/client/testing'
import { SLOW, captureInput, meMock, renderSection } from '@/test/page-harness'
import { ChannelDocument, UpdateChannelDocument } from '@/features/channels/Channels.gql'
import { CurrenciesPickerDocument } from '@/features/currencies/Currencies.gql'
import { IdentityEditor } from './IdentityEditor'

// What these pin: a bad public URL keeps the confirm dialog open with the reason, the update sends
// the fields this section doesn't own back unchanged, and a currency guard reads as advice

const channel = {
  __typename: 'Channel' as const,
  id: 'c1',
  slug: 'wheels',
  name: 'Wheels',
  defaultLocale: 'en',
  locales: [],
  isActive: true,
  isDefault: true,
  currencyCode: 'EUR',
  publicUrl: null,
  metadata: [{ __typename: 'MetadataEntry' as const, key: 'region', value: 'eu' }],
  createdAt: '2026-01-01T00:00:00Z',
}

function mocks(update: MockLink.MockedResponse): MockLink.MockedResponse[] {
  return [
    meMock(),
    { request: { query: ChannelDocument, variables: { id: 'c1' } }, result: { data: { channel } }, maxUsageCount: 3 },
    {
      request: { query: CurrenciesPickerDocument },
      result: { data: { currencies: [{ __typename: 'Currency', code: 'EUR', name: 'Euro' }] } },
    },
    update,
  ]
}

describe('IdentityEditor', () => {
  it('refuses a relative public URL, then saves the whole channel back', async () => {
    const onUpdate = vi.fn()
    renderSection(
      <IdentityEditor channelId="c1" />,
      mocks({
        request: { query: UpdateChannelDocument, variables: captureInput(onUpdate) },
        result: { data: { updateChannel: { ...channel, name: 'Wheels EU' } } },
      }),
    )

    fireEvent.change(await screen.findByLabelText('Name'), { target: { value: 'Wheels EU' } })
    fireEvent.change(screen.getByLabelText('Public URL'), { target: { value: 'wheels.example.com' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    const dialog = await screen.findByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply/ }))
    expect(await within(dialog).findByText(/An absolute URL/)).toBeDefined()
    expect(onUpdate).not.toHaveBeenCalled()

    fireEvent.change(screen.getByLabelText('Public URL'), { target: { value: 'https://wheels.example.com' } })
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply/ }))
    await vi.waitFor(
      () =>
        expect(onUpdate).toHaveBeenCalledWith({
          id: 'c1',
          name: 'Wheels EU',
          defaultLocale: 'en',
          isActive: true,
          currencyCode: 'EUR',
          publicUrl: 'https://wheels.example.com',
          metadata: [expect.objectContaining({ key: 'region', value: 'eu' })],
        }),
      SLOW,
    )
  })

  it('explains a blocked currency change instead of showing the error code', async () => {
    renderSection(
      <IdentityEditor channelId="c1" />,
      mocks({ request: { query: UpdateChannelDocument, variables: () => true }, error: new Error('STORE_CURRENCY_BLOCKED') }),
    )

    fireEvent.change(await screen.findByLabelText('Name'), { target: { value: 'Wheels EU' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    const dialog = await screen.findByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply/ }))
    expect(await within(dialog).findByText(/Set explicit prices first/, {}, SLOW)).toBeDefined()
  })
})
