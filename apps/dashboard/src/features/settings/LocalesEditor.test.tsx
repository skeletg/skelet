import { fireEvent, screen, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, meMock, renderSection } from '@/test/page-harness'
import { ChannelDocument } from '@/features/channels/Channels.gql'
import { LocalesPickerDocument, SetChannelLocalesDocument } from '@/features/locales/Locales.gql'
import { LocalesEditor } from './LocalesEditor'

// What these pin: the default locale can't be unticked out from under the channel, and a staged
// addition is reviewed and written through the page's Save

const channel = {
  __typename: 'Channel' as const,
  id: 'c1',
  slug: 'wheels',
  name: 'Wheels',
  defaultLocale: 'en',
  locales: [{ __typename: 'Locale' as const, code: 'en', name: 'English', nativeName: 'English', isActive: true, isRTL: false, sortOrder: 0 }],
  isActive: true,
  isDefault: true,
  currencyCode: 'EUR',
  publicUrl: null,
  metadata: [],
  createdAt: '2026-01-01T00:00:00Z',
}

const locale = (code: string, name: string) => ({ __typename: 'Locale' as const, code, name, nativeName: name, flagEmoji: '' })

describe('LocalesEditor', () => {
  it('refuses to drop the default, then adds a locale through the page Save', async () => {
    const onSet = vi.fn()
    renderSection(<LocalesEditor channelId="c1" />, [
      meMock(),
      { request: { query: ChannelDocument, variables: { id: 'c1' } }, result: { data: { channel } }, maxUsageCount: 3 },
      { request: { query: LocalesPickerDocument }, result: { data: { locales: [locale('en', 'English'), locale('de', 'Deutsch')] } } },
      {
        request: {
          query: SetChannelLocalesDocument,
          variables: (vars: Record<string, unknown>) => {
            onSet(vars)
            return true
          },
        },
        result: { data: { setChannelLocales: { __typename: 'Channel', id: 'c1', locales: [] } } },
      },
    ])

    fireEvent.click(await screen.findByRole('checkbox', { name: 'Support en' }))
    expect(await screen.findByText(/is the channel's default locale/)).toBeDefined()

    fireEvent.click(screen.getByRole('checkbox', { name: 'Support de' }))
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    const dialog = await screen.findByRole('alertdialog')
    expect(within(dialog).getByText('Locale · de')).toBeDefined()
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply 1 change/ }))
    await vi.waitFor(() => expect(onSet).toHaveBeenCalledWith({ channelId: 'c1', codes: ['en', 'de'] }), SLOW)
  })
})
