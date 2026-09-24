import { fireEvent, screen, within } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { SLOW, meMock, renderPage } from '@/test/page-harness'
import { AttributesDocument, SetAttributeIndexableDocument } from '@/features/catalog/Catalog.gql'
import { ConfigSectionsDocument, SetConfigValueDocument } from '@/features/settings/Settings.gql'
import { FacetSeoPage } from './FacetSeoPage'

// What these pin: a combination row with an empty slot is refused instead of silently dropped, and
// a facet switch is written without touching the stored combinations

const attr = (id: string, slug: string, name: string) => ({
  __typename: 'Attribute' as const,
  id,
  slug,
  name,
  inputType: 'DROPDOWN' as const,
  relationshipTarget: null,
  unit: '',
  valueRequired: false,
  filterable: true,
  searchable: false,
  isPublic: true,
  facetCollapsed: false,
  isIndexable: true,
  h1Position: 'BEFORE' as const,
  synapseTrack: false,
  autoSegment: false,
  sortOrder: 0,
  defaultValue: null,
  values: [],
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
})

const combos = {
  __typename: 'ConfigField' as const,
  path: 'seo/facets/indexable_combinations',
  section: 'seo',
  group: 'facets',
  field: 'indexable_combinations',
  label: 'Indexable combinations',
  description: '',
  type: 'TEXTAREA',
  value: 'brand + size',
  isSet: true,
  options: [],
  isDefault: false,
  public: false,
  scoping: 'CHANNEL' as const,
  scope: { __typename: 'ConfigScope' as const, kind: 'DEFAULT' as const, channelId: null, locale: null },
  updatedAt: null,
}

const recorder = (fn: (v: Record<string, unknown>) => void) => (vars: Record<string, unknown>) => {
  fn(vars)
  return true
}

afterEach(() => localStorage.clear())

describe('FacetSeoPage', () => {
  it('refuses a half-filled combination, then writes just the facet switch', async () => {
    const writes = { facet: vi.fn(), config: vi.fn() }
    renderPage({
      path: '/seo/facets',
      adminId: 'admin-1',
      routes: { '/seo/facets': <FacetSeoPage /> },
      mocks: [
        meMock(),
        {
          request: { query: ConfigSectionsDocument, variables: () => true },
          result: {
            data: {
              configSections: [
                {
                  __typename: 'ConfigSection',
                  slug: 'seo',
                  label: 'SEO',
                  groups: [{ __typename: 'ConfigGroup', slug: 'facets', label: 'Facets', fields: [combos] }],
                },
              ],
            },
          },
          maxUsageCount: 3,
        },
        { request: { query: AttributesDocument }, result: { data: { attributes: [attr('a1', 'brand', 'Brand'), attr('a2', 'size', 'Size')] } }, maxUsageCount: 3 },
        {
          request: { query: SetAttributeIndexableDocument, variables: recorder(writes.facet) },
          result: { data: { setAttributeIndexable: { __typename: 'SetAttributeIndexablePayload', attribute: { __typename: 'Attribute', id: 'a2', isIndexable: false, values: [] } } } },
        },
        { request: { query: SetConfigValueDocument, variables: recorder(writes.config) }, result: { data: { setConfigValue: null } } },
      ],
    })

    const save = await screen.findByRole('button', { name: 'Save changes' }, SLOW)
    fireEvent.click(screen.getByRole('switch', { name: 'Facet Size indexable' }))
    fireEvent.click(screen.getByRole('button', { name: /Add combination/ }))
    fireEvent.click(save)
    let dialog = await screen.findByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply/ }))
    expect(await within(dialog).findByText(/Pick a facet for every slot/)).toBeDefined()
    expect(writes.facet).not.toHaveBeenCalled()

    fireEvent.click(within(dialog).getByRole('button', { name: 'Cancel' }))
    fireEvent.click(screen.getAllByRole('button', { name: 'Remove combination' })[1])
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    dialog = await screen.findByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply 1 change/ }))
    await vi.waitFor(() => expect(writes.facet).toHaveBeenCalledWith({ attributeId: 'a2', isIndexable: false }), SLOW)
    expect(writes.config).not.toHaveBeenCalled()
  })
})
