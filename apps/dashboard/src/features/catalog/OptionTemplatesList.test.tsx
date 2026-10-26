import { screen, within } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { SLOW, meMock, renderPage } from '@/test/page-harness'
import { OptionTemplatesDocument } from './Catalog.gql'
import { OptionTemplatesList } from './OptionTemplatesList'

// What this pins: the template list says what each template reaches - products picking it and product types handing it
// over - and offers a new one only to a store-wide catalog manager

const summary = (id: string, name: string, productCount: number, productTypeCount: number) => ({
  __typename: 'OptionTemplate' as const, id, name, description: '', currencyCode: 'USD', optionCount: 1, ruleCount: 0,
  productCount, productTypeCount, updatedAt: '2026-10-01T00:00:00Z',
})

function open(me = meMock()) {
  renderPage({
    path: '/catalog/configurator-templates',
    adminId: 'admin-1',
    routes: { '/catalog/configurator-templates': <OptionTemplatesList /> },
    mocks: [
      me,
      {
        request: { query: OptionTemplatesDocument },
        result: { data: { optionTemplates: [summary('t1', 'Gift wrapping', 3, 1), summary('t2', 'Care plan', 0, 2), summary('t3', 'Monogram', 1, 0), summary('t4', 'Unused', 0, 0)] } },
        maxUsageCount: 5,
      },
    ],
  })
}

const row = (name: string) => screen.getByRole('link', { name }).closest('tr') as HTMLElement

describe('OptionTemplatesList', () => {
  it('says what each template reaches', async () => {
    open()
    await screen.findByRole('link', { name: 'Gift wrapping' }, SLOW)
    expect(within(row('Gift wrapping')).getByText('3 products · 1 product type')).toBeDefined()
    expect(within(row('Care plan')).getByText('2 product types')).toBeDefined()
    expect(within(row('Monogram')).getByText('1 product')).toBeDefined()
    expect(within(row('Unused')).getAllByText('-').length).toBeGreaterThan(0)
    expect(row('Gift wrapping').querySelector('a')?.getAttribute('href')).toBe('/catalog/configurator-templates/t1')
  })

  it('offers a new template to a store-wide catalog manager only', async () => {
    open()
    expect(await screen.findByRole('button', { name: 'New template' }, SLOW)).toBeDefined()
  })

  it('offers none to an admin without catalog.manage everywhere', async () => {
    open(meMock({ superuser: false, permissions: ['catalog.view'] }))
    await screen.findByRole('link', { name: 'Gift wrapping' }, SLOW)
    expect(screen.queryByRole('button', { name: 'New template' })).toBeNull()
  })
})
