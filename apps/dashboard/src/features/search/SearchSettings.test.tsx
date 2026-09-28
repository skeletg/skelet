import { fireEvent, screen, within } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { SLOW, meMock, renderPage } from '@/test/page-harness'
import { AttributesDocument } from '@/features/catalog/Catalog.gql'
import { ConfigSectionsDocument, SetConfigValueDocument } from '@/features/settings/Settings.gql'
import { SearchSettings } from './SearchSettings'

// What these pin: search settings stage and save through the standard confirm dialog like every
// other config page, broken JSON keeps the dialog open with the reason, and only changed values go

const field = (path: string, label: string, type: string, value: string, group: string) => ({
  __typename: 'ConfigField' as const,
  path,
  section: 'search',
  group,
  field: path.split('/').pop(),
  label,
  description: '',
  type,
  value,
  isSet: true,
  options: [],
  isDefault: true,
  public: false,
  scoping: 'GLOBAL' as const,
  scope: { __typename: 'ConfigScope' as const, kind: 'DEFAULT' as const, channelId: null, locale: null },
  updatedAt: null,
})

const section = {
  __typename: 'ConfigSection' as const,
  slug: 'search',
  label: 'Search',
  groups: [
    { __typename: 'ConfigGroup' as const, slug: 'general', label: 'General', fields: [field('search/general/max_page_size', 'Max page size', 'INT', '100', 'general')] },
    { __typename: 'ConfigGroup' as const, slug: 'mapping', label: 'Mapping', fields: [field('search/mapping/analyzers', 'Analyzers', 'JSON', '{}', 'mapping')] },
  ],
}

afterEach(() => localStorage.clear())

describe('SearchSettings', () => {
  it('keeps the dialog open on broken JSON, then writes only the changed value', async () => {
    const writes = vi.fn()
    renderPage({
      path: '/search',
      adminId: 'admin-1',
      routes: { '/search': <SearchSettings /> },
      mocks: [
        meMock(),
        { request: { query: ConfigSectionsDocument, variables: { scope: { kind: 'DEFAULT' } } }, result: { data: { configSections: [section] } }, maxUsageCount: 3 },
        { request: { query: AttributesDocument }, result: { data: { attributes: [] } } },
        {
          request: {
            query: SetConfigValueDocument,
            variables: (vars: Record<string, unknown>) => {
              writes(vars)
              return true
            },
          },
          result: {
            data: {
              setConfigValue: {
                __typename: 'ConfigValue',
                path: 'search/general/max_page_size',
                value: '50',
                isDefault: false,
                scope: { __typename: 'ConfigScope', kind: 'DEFAULT', channelId: null, locale: null },
                updatedAt: null,
              },
            },
          },
        },
      ],
    })

    // ConfigField ignores changes to an unfocused input (its autofill guard) - type the way a person would
    const type = (el: HTMLElement, value: string) => {
      el.focus()
      fireEvent.change(el, { target: { value } })
    }
    type(await screen.findByLabelText('Max page size'), '50')
    type(screen.getByLabelText('Analyzers'), '{ broken')
    expect(screen.getByText(/reindex required/)).toBeDefined()
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    const dialog = await screen.findByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply/ }))
    expect(await within(dialog).findByText(/isn't valid JSON/)).toBeDefined()
    expect(writes).not.toHaveBeenCalled()

    // The dialog is modal - back out, fix the field, review again
    fireEvent.click(within(dialog).getByRole('button', { name: 'Cancel' }))
    type(screen.getByLabelText('Analyzers'), '{}')
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    fireEvent.click(within(await screen.findByRole('alertdialog')).getByRole('button', { name: /Apply/ }))
    await vi.waitFor(() => expect(writes).toHaveBeenCalledTimes(1), SLOW)
    expect(writes).toHaveBeenCalledWith(expect.objectContaining({ path: 'search/general/max_page_size', value: '50' }))
  })
})
