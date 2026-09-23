import { fireEvent, render, screen, within } from '@testing-library/react'
import { MockedProvider } from '@apollo/client/testing/react'
import { GraphQLError } from 'graphql'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import { CountriesPickerDocument } from '@/features/countries/Countries.gql'
import { LocaleEditPage } from './LocaleForm'
import { LocaleDocument, LocalesAdminListDocument, UpdateLocaleDocument } from './Locales.gql'

// Save -> mutate -> navigate is several async hops - past testing-library's 1s default when the whole
// suite is loading the machine, which made these flaky
const SLOW = { timeout: 5000 }

// Through the real page: an edit lights the header Save (it never did), confirming sends the
// record, and a service rejection naming a field keeps the dialog open and shows under that field

const stored = {
  __typename: 'Locale' as const,
  code: 'es-mx',
  name: 'Spanish (Mexico)',
  nativeName: 'Español (México)',
  description: '',
  isActive: true,
  isRTL: false,
  sortOrder: 0,
  flagEmoji: '',
  dateFormat: '',
  timeFormat: '',
  decimalSeparator: '.',
  thousandsSeparator: ',',
  firstDayOfWeek: 1,
  defaultCountryCode: 'MX',
  hreflangValue: '',
  script: '',
  pluralRule: '',
  emailFromNameOverride: '',
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
  metadata: [],
}

function renderPage(update: { ok: boolean }, onWrite = vi.fn()) {
  const mocks = [
    { request: { query: LocaleDocument, variables: { code: 'es-mx' } }, result: { data: { locale: stored } } },
    { request: { query: CountriesPickerDocument }, result: { data: { countries: [] } } },
    {
      request: {
        query: UpdateLocaleDocument,
        variables: (vars: { input: Record<string, unknown> }) => {
          onWrite(vars.input)
          return true
        },
      },
      result: update.ok
        ? { data: { updateLocale: { ...stored } } }
        : { errors: [new GraphQLError('hreflang_value "xx_YY" is not a valid BCP 47 tag')] },
    },
    { request: { query: LocalesAdminListDocument }, result: { data: { locales: [] } }, maxUsageCount: 5 },
  ]
  render(
    <MockedProvider mocks={mocks}>
      <MemoryRouter initialEntries={['/locales/es-mx']}>
        <Routes>
          <Route path="/locales/:code" element={<LocaleEditPage />} />
          <Route path="/locales" element={<p>locales list</p>} />
        </Routes>
      </MemoryRouter>
    </MockedProvider>,
  )
  return onWrite
}

async function editHreflangAndConfirm() {
  fireEvent.change(await screen.findByLabelText('hreflang value'), { target: { value: 'xx_YY' } })
  const save = screen.getByRole('button', { name: 'Save changes' }) as HTMLButtonElement
  expect(save.disabled).toBe(false)
  fireEvent.click(save)
  const dialog = await screen.findByRole('alertdialog')
  expect(within(dialog).getByText('hreflang value')).toBeDefined()
  fireEvent.click(within(dialog).getByRole('button', { name: /Apply 1 change/ }))
  return dialog
}

describe('LocaleEditPage', () => {
  it('saves a reviewed edit', async () => {
    const onWrite = renderPage({ ok: true })
    await editHreflangAndConfirm()
    await screen.findByText('locales list', {}, SLOW)
    expect(onWrite).toHaveBeenCalledWith(expect.objectContaining({ code: 'es-mx', hreflangValue: 'xx_YY' }))
  })

  it('routes a service rejection to its field and keeps the dialog open', async () => {
    renderPage({ ok: false })
    const dialog = await editHreflangAndConfirm()
    expect(await within(dialog).findByText(/not a valid BCP 47 tag/)).toBeDefined()
    expect(await screen.findByText("Doesn't look like a BCP 47 code.")).toBeDefined()
  })
})
