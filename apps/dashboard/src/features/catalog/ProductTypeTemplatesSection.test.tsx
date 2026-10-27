import { fireEvent, screen, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, captureInput, renderPage } from '@/test/page-harness'
import { OptionTemplatesDocument, SetProductTypeOptionTemplatesDocument } from './Catalog.gql'
import { ProductTypeTemplatesSection } from './ProductTypeTemplatesSection'

// What these pin: a type's templates list in the order its products show them, Show earlier/later swaps two, a
// template added from the menu goes last, removing one asks first, a refusal from the server is shown, and without
// store-wide catalog.manage the list only shows

Element.prototype.hasPointerCapture ??= () => false
Element.prototype.releasePointerCapture ??= () => {}
Element.prototype.scrollIntoView ??= () => {}

const ref = (id: string, name: string) => ({ __typename: 'OptionTemplate' as const, id, name, description: '', optionCount: 1, ruleCount: 0 })
const summary = (id: string, name: string) => ({
  ...ref(id, name), currencyCode: 'USD', productCount: 0, productTypeCount: 0, updatedAt: '2026-10-01T00:00:00Z',
})
const mugs = { id: 'pt1', name: 'Mugs', optionTemplates: [ref('t1', 'Care plan'), ref('t2', 'Gift wrap'), ref('t3', 'Engraving')] }

function rig({ canManage = true, refuse = false } = {}) {
  const sent = vi.fn()
  renderPage({
    path: '/catalog/product-types/pt1',
    routes: { '/catalog/product-types/pt1': <ProductTypeTemplatesSection productType={mugs} canManage={canManage} /> },
    mocks: [
      { request: { query: OptionTemplatesDocument }, result: { data: { optionTemplates: [summary('t1', 'Care plan'), summary('t4', 'Monogram')] } }, maxUsageCount: 5 },
      refuse
        ? { request: { query: SetProductTypeOptionTemplatesDocument, variables: captureInput(sent) }, error: new Error('option in use: a rule on product "MUG-1" uses it') }
        : {
            request: { query: SetProductTypeOptionTemplatesDocument, variables: captureInput(sent) },
            result: { data: { setProductTypeOptionTemplates: { __typename: 'ProductType' as const, id: 'pt1', optionTemplates: [] } } },
          },
    ],
  })
  return { sent }
}

const rows = () => within(screen.getByRole('list', { name: 'Templates for this type' })).getAllByRole('listitem')

describe('ProductTypeTemplatesSection', () => {
  it('lists the templates in order and swaps one earlier', async () => {
    const { sent } = rig()
    expect(rows().map((r) => within(r).getByRole('link').textContent)).toEqual(['Care plan', 'Gift wrap', 'Engraving'])
    expect(within(rows()[0]).getByRole('link').getAttribute('href')).toBe('/catalog/configurator-templates/t1')
    expect(screen.getByRole('button', { name: 'Show Care plan earlier' })).toHaveProperty('disabled', true)
    expect(screen.getByRole('button', { name: 'Show Engraving later' })).toHaveProperty('disabled', true)
    fireEvent.click(screen.getByRole('button', { name: 'Show Engraving earlier' }))
    await vi.waitFor(() => expect(sent).toHaveBeenCalledWith({ productTypeId: 'pt1', templateIds: ['t1', 't3', 't2'] }), SLOW)
  })

  it('adds a template from the menu at the end, offering only the ones it lacks', async () => {
    const { sent } = rig()
    fireEvent.pointerDown(screen.getByRole('button', { name: 'Use a template' }), { button: 0, ctrlKey: false, pointerType: 'mouse' })
    fireEvent.click(await screen.findByRole('menuitem', { name: /Monogram/ }, SLOW))
    expect(screen.queryByRole('menuitem', { name: /Care plan/ })).toBeNull()
    await vi.waitFor(() => expect(sent).toHaveBeenCalledWith({ productTypeId: 'pt1', templateIds: ['t1', 't2', 't3', 't4'] }), SLOW)
  })

  it('asks before removing one, then sends the rest', async () => {
    const { sent } = rig()
    fireEvent.click(screen.getByRole('button', { name: 'Remove Gift wrap' }))
    const confirm = await screen.findByRole('alertdialog', {}, SLOW)
    expect(within(confirm).getByText('Remove Gift wrap from Mugs?')).toBeDefined()
    expect(sent).not.toHaveBeenCalled()
    fireEvent.click(within(confirm).getByRole('button', { name: 'Remove' }))
    await vi.waitFor(() => expect(sent).toHaveBeenCalledWith({ productTypeId: 'pt1', templateIds: ['t1', 't3'] }), SLOW)
  })

  it("shows why the server refused", async () => {
    rig({ refuse: true })
    fireEvent.click(screen.getByRole('button', { name: 'Show Gift wrap earlier' }))
    expect(await screen.findByText(/a rule on product "MUG-1" uses it/, undefined, SLOW)).toBeDefined()
  })

  it('says why a template could not be used in the dialog alone', async () => {
    rig({ refuse: true })
    fireEvent.pointerDown(screen.getByRole('button', { name: 'Use a template' }), { button: 0, ctrlKey: false, pointerType: 'mouse' })
    fireEvent.click(await screen.findByRole('menuitem', { name: 'Manage templates…' }, SLOW))
    const dialog = await screen.findByRole('dialog', { name: 'Configurator templates' }, SLOW)
    fireEvent.click(await within(dialog).findByRole('button', { name: 'Use here' }, SLOW))
    expect(await within(dialog).findByText(/a rule on product "MUG-1" uses it/, undefined, SLOW)).toBeDefined()
    expect(screen.getAllByText(/a rule on product "MUG-1" uses it/)).toHaveLength(1)
  })

  it('only shows the list without store-wide catalog.manage', () => {
    rig({ canManage: false })
    expect(rows()).toHaveLength(3)
    expect(screen.queryByRole('button', { name: 'Use a template' })).toBeNull()
    expect(screen.queryByRole('button', { name: /^Remove / })).toBeNull()
    expect(screen.queryByRole('button', { name: /earlier|later/ })).toBeNull()
  })
})
