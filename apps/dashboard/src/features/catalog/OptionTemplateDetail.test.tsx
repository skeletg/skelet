import { screen, within } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { SLOW, meMock, renderDialog } from '@/test/page-harness'
import { ChannelsDocument } from '@/features/channels/Channels.gql'
import { CurrenciesPickerDocument } from '@/features/currencies/Currencies.gql'
import { OptionTemplateDocument, OptionTemplateProductsDocument } from './Catalog.gql'
import { OptionTemplateDetail } from './OptionTemplateDetail'

// What these pin: a template opened from a product or a product type goes Back there, and to the template list
// otherwise; the product types handing it over are listed with the way to them; and it can't be deleted while a type
// still uses it, even with no product picking it

Element.prototype.hasPointerCapture ??= () => false
Element.prototype.releasePointerCapture ??= () => {}
Element.prototype.scrollIntoView ??= () => {}

const template = (over: object = {}) => ({
  __typename: 'OptionTemplate' as const, id: 't1', name: 'Care plan', description: '', currencyCode: 'USD', optionCount: 0, ruleCount: 0,
  productCount: 0, productTypeCount: 1, updatedAt: '2026-10-01T00:00:00Z', productOptions: [], optionRules: [],
  productTypes: [{ __typename: 'ProductType' as const, id: 'pt1', name: 'Mugs' }],
  ...over,
})

function open(state: unknown, value = template()) {
  renderDialog(
    <MemoryRouter initialEntries={[{ pathname: '/catalog/configurator-templates/t1', state }]}>
      <Routes>
        <Route path="/catalog/configurator-templates/:id" element={<OptionTemplateDetail />} />
      </Routes>
    </MemoryRouter>,
    [
      meMock(),
      { request: { query: OptionTemplateDocument, variables: { id: 't1' } }, result: { data: { optionTemplate: value } }, maxUsageCount: 5 },
      {
        request: { query: OptionTemplateProductsDocument, variables: () => true },
        result: { data: { optionTemplate: { __typename: 'OptionTemplate' as const, id: 't1', productCount: value.productCount, products: [] } } },
        maxUsageCount: 5,
      },
      { request: { query: ChannelsDocument }, result: { data: { channels: [] } }, maxUsageCount: 5 },
      { request: { query: CurrenciesPickerDocument }, result: { data: { currencies: [] } }, maxUsageCount: 5 },
    ],
  )
}

describe('OptionTemplateDetail', () => {
  it('goes back to the product it was opened from', async () => {
    open({ from: '/products/p1' })
    const back = await screen.findByRole('link', { name: 'Back to the product' }, SLOW)
    expect(back.getAttribute('href')).toBe('/products/p1')
  })

  it('goes back to the product type it was opened from', async () => {
    open({ from: '/catalog/product-types/pt1' })
    expect((await screen.findByRole('link', { name: 'Back to the product type' }, SLOW)).getAttribute('href')).toBe('/catalog/product-types/pt1')
  })

  it.each(['https://elsewhere.example', '//elsewhere.example/products/p1'])('goes back to the template list for %s - never off the app', async (from) => {
    open({ from })
    expect((await screen.findByRole('link', { name: 'Back to configurator templates' }, SLOW)).getAttribute('href')).toBe('/catalog/configurator-templates')
  })

  it('lists the product types handing it over and keeps Delete off while one does', async () => {
    open(null)
    const types = await screen.findByRole('list', { name: 'Product types handing it over' }, SLOW)
    expect(within(types).getByRole('link', { name: 'Mugs' }).getAttribute('href')).toBe('/catalog/product-types/pt1')
    expect(screen.getByText('Used by 1 product type')).toBeDefined()
    expect(await screen.findByRole('button', { name: 'Delete' }, SLOW)).toHaveProperty('disabled', true)
    // the products load on their own query - let it answer before saying the empty note stays away
    await new Promise((r) => setTimeout(r, 50))
    expect(screen.queryByText(/No product uses it yet/)).toBeNull()
  })

  it('lets an unused template be deleted', async () => {
    open(null, template({ productTypeCount: 0, productTypes: [] }))
    expect(await screen.findByText('Not used yet', undefined, SLOW)).toBeDefined()
    // Delete shows once the grants arrive
    expect(await screen.findByRole('button', { name: 'Delete' }, SLOW)).toHaveProperty('disabled', false)
    expect(await screen.findByText(/No product uses it yet/, undefined, SLOW)).toBeDefined()
  })
})
