import { act, fireEvent, screen, waitFor, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, captureInput, renderSection } from '@/test/page-harness'
import { PrefsContext } from '@/lib/admin-prefs'
import { ProductPickerPagedDocument, ProductTypesDocument, ProductVariantsDocument, SetBundlePricingDocument, UpdateProductDocument } from './Catalog.gql'
import { BundleStructureSection } from './BundleStructureSection'
import { GroupedStructureSection } from './GroupedStructureSection'
import { KitStructureSection } from './KitStructureSection'

// What this pins: each structure section's remove confirm runs through the shared ConfirmDialog - confirming
// only stages the change, so the dialog closes at once and the header save picks it up

const widget = { id: 'p2', sku: 'W-1', name: 'Widget', kind: 'SIMPLE', defaultPrice: 500, defaultCurrencyCode: 'EUR' }

async function confirmIn(label: string, confirm: string) {
  fireEvent.click(await screen.findByRole('button', { name: label }))
  const dialog = await screen.findByRole('alertdialog')
  fireEvent.click(within(dialog).getByRole('button', { name: confirm }))
  await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
  expect((screen.getByRole('button', { name: 'Save changes' }) as HTMLButtonElement).disabled).toBe(false)
}

describe('structure section remove confirms', () => {
  it('stages a grouped member removal', async () => {
    renderSection(<GroupedStructureSection canManage product={{ id: 'p1', groupedMembers: [{ id: 'gm1', member: widget, defaultQuantity: 1, sortOrder: 0 }] }} />)
    await confirmIn('Remove member', 'Stage remove')
  })

  it('stages a kit component removal', async () => {
    renderSection(<KitStructureSection canManage product={{ id: 'p1', kitItemsReturnable: false, kitComponents: [{ id: 'kc1', component: widget, quantity: 1, sortOrder: 0 }] }} />)
    await confirmIn('Remove component', 'Stage remove')
  })

  it('stages a bundle option delete and a selection removal', async () => {
    const selection = { id: 's1', product: widget, quantity: 1, isDefault: false, sortOrder: 0 }
    const option = { id: 'o1', inputType: 'RADIO' as const, isRequired: true, sortOrder: 0, minSelections: 0, maxSelections: null, title: 'Size', content: [{ locale: 'en', title: 'Size' }], selections: [selection] }
    renderSection(<BundleStructureSection canManage product={{ id: 'p1', bundleOptions: [option] }} />)
    await confirmIn('Remove selection', 'Stage remove')
    await confirmIn('Delete option', 'Stage delete')
  })
})

// What this pins: the option title is edited in the page's content locale - a translated view must neither show nor
// overwrite whichever locale happens to be stored first
describe('bundle option title locale', () => {
  const option = {
    id: 'o1', inputType: 'RADIO' as const, isRequired: true, sortOrder: 0, minSelections: 0, maxSelections: null, title: 'Veličina',
    content: [{ locale: 'en', title: 'Size' }, { locale: 'sr', title: 'Veličina' }], selections: [],
  }
  const editTitle = async (contentLocale: string) => {
    renderSection(<BundleStructureSection canManage product={{ id: 'p1', contentLocale, bundleOptions: [option] }} />)
    fireEvent.click(await screen.findByRole('button', { name: 'Edit option' }))
    return (await screen.findByLabelText(`Title (${contentLocale})`)) as HTMLInputElement
  }

  it("edits the page locale's own title", async () => {
    expect((await editTitle('sr')).value).toBe('Veličina')
  })

  it('starts an untranslated locale from the fallback title', async () => {
    expect((await editTitle('de')).value).toBe('Veličina')
  })

  it('stages a translation removal for the page save instead of deleting it at once', async () => {
    await editTitle('sr')
    await confirmIn('Remove translation', 'Stage remove')
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  })
})

// What this pins: pick limits are only offered where they mean something, and a contradictory pair can't be staged
describe('bundle option pick limits', () => {
  const openEdit = async (inputType: 'RADIO' | 'CHECKBOX') => {
    const option = { id: 'o1', inputType, isRequired: false, sortOrder: 0, minSelections: 0, maxSelections: null, title: 'Shirts', content: [{ locale: 'en', title: 'Shirts' }], selections: [] }
    renderSection(<BundleStructureSection canManage product={{ id: 'p1', bundleOptions: [option] }} />)
    fireEvent.click(await screen.findByRole('button', { name: 'Edit option' }))
    await screen.findByRole('dialog')
  }

  it('hides the limits on a single-select option', async () => {
    await openEdit('RADIO')
    expect(screen.queryByLabelText('At least')).toBeNull()
  })

  it('refuses at most below at least', async () => {
    await openEdit('CHECKBOX')
    fireEvent.change(screen.getByLabelText('At least'), { target: { value: '3' } })
    fireEvent.change(screen.getByLabelText('At most'), { target: { value: '2' } })
    expect(screen.getByText('At most can’t be lower than at least.')).toBeTruthy()
    expect((screen.getByRole('button', { name: 'Stage edit' }) as HTMLButtonElement).disabled).toBe(true)
  })

  it('warns when the minimum outruns the selections', async () => {
    await openEdit('CHECKBOX')
    fireEvent.change(screen.getByLabelText('At least'), { target: { value: '3' } })
    expect(screen.getByText(/customers can’t pick 3 until you add more/)).toBeTruthy()
  })
})

// What this pins: the health strip names the store and what's wrong there, and prices the bundle per store
describe('bundle health strip', () => {
  it('flags an out-of-stock component and shows the store price', async () => {
    const listings = [{ isPublished: true, channel: { id: 'c1', name: 'EU store', currencyCode: 'EUR' } }]
    const stocked = { ...widget, listings: [{ channelId: 'c1', isPublished: true, effectivePrice: 500, inStock: true }] }
    const gone = { ...widget, id: 'p3', name: 'Gadget', listings: [{ channelId: 'c1', isPublished: true, effectivePrice: 900, inStock: false }] }
    const option = {
      id: 'o1', inputType: 'RADIO' as const, isRequired: true, sortOrder: 0, minSelections: 0, maxSelections: null, title: 'Size', content: [],
      selections: [{ id: 's1', product: stocked, quantity: 1, isDefault: true, sortOrder: 0 }, { id: 's2', product: gone, quantity: 1, isDefault: false, sortOrder: 1 }],
    }
    renderSection(<BundleStructureSection canManage product={{ id: 'p1', listings, bundleOptions: [option] }} />)
    const strip = await screen.findByLabelText('Bundle health')
    expect(within(strip).getByText('EU store')).toBeTruthy()
    expect(within(strip).getByText('Gadget in “Size” is out of stock.')).toBeTruthy()
    expect(strip.textContent).toMatch(/Customers pay .*5\.00.*, default .*5\.00/)
  })
})

// What this pins: a store's bundle discount stages for the page Save, moves the price line at once, and saves only that store
describe('bundle pricing per store', () => {
  it('stages a new discount and saves it on Save', async () => {
    const set = vi.fn()
    const listings = [{ isPublished: true, effectivePrice: null, bundlePricing: 'SUM_MINUS_PERCENT' as const, bundleDiscountPercent: 10, channel: { id: 'c1', name: 'EU store', currencyCode: 'EUR' } }]
    const mug = { ...widget, listings: [{ channelId: 'c1', isPublished: true, effectivePrice: 1000, inStock: true }] }
    const option = {
      id: 'o1', inputType: 'RADIO' as const, isRequired: true, sortOrder: 0, minSelections: 0, maxSelections: null, title: 'Size', content: [],
      selections: [{ id: 's1', product: mug, quantity: 1, isDefault: true, sortOrder: 0 }],
    }
    renderSection(<BundleStructureSection canManage product={{ id: 'p1', listings, bundleOptions: [option] }} />, [
      { request: { query: SetBundlePricingDocument, variables: captureInput(set) }, result: { data: { setBundlePricing: { __typename: 'Product', id: 'p1' } } } },
    ])
    const strip = await screen.findByLabelText('Bundle health')
    expect(strip.textContent).toMatch(/Customers pay .*9\.00/)
    fireEvent.change(within(strip).getByLabelText('EU store discount percent'), { target: { value: '25' } })
    expect(strip.textContent).toMatch(/Customers pay .*7\.50.*picks add up to .*10\.00/)
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    const dialog = await screen.findByRole('alertdialog')
    expect(within(dialog).getByText('25% off the picks')).toBeTruthy()
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply/ }))
    await vi.waitFor(() => expect(set).toHaveBeenCalledWith({ productId: 'p1', channelId: 'c1', pricing: 'SUM_MINUS_PERCENT', discountPercent: 25 }), SLOW)
  })
})

// What this pins: whether a kit's items come back one at a time is staged like its components and applied by the page Save
describe('kit item returns', () => {
  it('stages the setting and saves it on Save', async () => {
    const update = vi.fn()
    renderSection(<KitStructureSection canManage product={{ id: 'p1', kitItemsReturnable: false, kitComponents: [{ id: 'kc1', component: widget, quantity: 1, sortOrder: 0 }] }} />, [
      { request: { query: UpdateProductDocument, variables: captureInput(update) }, result: { data: { updateProduct: { __typename: 'Product', id: 'p1', kitItemsReturnable: true } } } },
    ])
    fireEvent.click(await screen.findByRole('switch', { name: 'Return items separately' }))
    expect(update).not.toHaveBeenCalled()
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    const dialog = await screen.findByRole('alertdialog')
    expect(within(dialog).getByText('Whole kit or single items')).toBeTruthy()
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply/ }))
    await vi.waitFor(() => expect(update).toHaveBeenCalledWith({ id: 'p1', kitItemsReturnable: true }), SLOW)
  })
})

// What this pins: quantity, default and order are edited in the row itself and staged for the page Save - a pick-one
// option's new default takes the old one's place
describe('bundle selection rows edit in place', () => {
  const mk = (id: string, name: string, isDefault: boolean, sortOrder: number) => ({ id, product: { ...widget, id: `p-${id}`, name }, quantity: 1, isDefault, sortOrder })
  const render = () =>
    renderSection(
      <BundleStructureSection
        canManage
        product={{
          id: 'p1',
          bundleOptions: [{
            id: 'o1', inputType: 'RADIO' as const, isRequired: true, sortOrder: 0, minSelections: 0, maxSelections: null, title: 'Size', content: [],
            selections: [mk('s1', 'Small', true, 0), mk('s2', 'Large', false, 1)],
          }],
        }}
      />,
    )
  const saveDialog = async () => {
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    return screen.findByRole('alertdialog')
  }

  it('stages a quantity typed into the row', async () => {
    render()
    fireEvent.change(await screen.findByLabelText('Large quantity'), { target: { value: '3' } })
    expect((screen.getByLabelText('Large quantity') as HTMLInputElement).value).toBe('3')
    expect((await saveDialog()).textContent).toMatch(/Selection · Large · qty/)
  })

  it('moves a pick-one default to the row switched on', async () => {
    render()
    fireEvent.click(await screen.findByRole('switch', { name: 'Large is a default' }))
    expect(screen.getByRole('switch', { name: 'Large is a default' }).getAttribute('aria-checked')).toBe('true')
    expect(screen.getByRole('switch', { name: 'Small is a default' }).getAttribute('aria-checked')).toBe('false')
    const text = (await saveDialog()).textContent
    expect(text).toMatch(/Selection · Large · default/)
    expect(text).toMatch(/Selection · Small · default/)
  })

  it('reorders by keyboard drag', async () => {
    // jsdom lays nothing out, so give each row a place for the sensor to measure - as RecommendationRows.test does
    const rect = vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function (this: HTMLElement) {
      const row = this.closest('tr')
      const index = row ? Array.from(row.parentElement!.children).indexOf(row) : 0
      return { x: 0, y: index * 80, top: index * 80, bottom: index * 80 + 80, left: 0, right: 300, width: 300, height: 80, toJSON: () => ({}) }
    })
    const height = vi.spyOn(HTMLElement.prototype, 'offsetHeight', 'get').mockReturnValue(80)
    const width = vi.spyOn(HTMLElement.prototype, 'offsetWidth', 'get').mockReturnValue(300)
    try {
      render()
      const handle = await screen.findByRole('button', { name: 'Reorder Small' })
      handle.focus()
      fireEvent.keyDown(handle, { key: ' ', code: 'Space' })
      await vi.waitFor(() => expect(handle.getAttribute('aria-pressed')).toBe('true'), SLOW)
      await act(() => new Promise<void>((resolve) => setTimeout(resolve, 0)))
      fireEvent.keyDown(handle, { key: 'ArrowDown', code: 'ArrowDown' })
      await act(() => new Promise<void>((resolve) => setTimeout(resolve, 0)))
      fireEvent.keyDown(handle, { key: ' ', code: 'Space' })
      const order = () => screen.getAllByRole('button', { name: /^Reorder / }).map((b) => b.getAttribute('aria-label'))
      await vi.waitFor(() => expect(order()).toEqual(['Reorder Large', 'Reorder Small']), SLOW)
      expect((await saveDialog()).textContent).toMatch(/Selection · Small · order/)
    } finally {
      rect.mockRestore()
      height.mockRestore()
      width.mockRestore()
    }
  })
})

// What this pins: a picked configurable stays one selection whose variant the customer picks, or - with Each variant
// ticked - is split into a selection per variant, so "any size" is one pick either way
describe('bundle picker and configurables', () => {
  const hoodie = {
    __typename: 'Product' as const, id: 'cfg', name: 'Hoodie', sku: 'HOODIE', kind: 'CONFIGURABLE' as const, defaultPrice: 4900, defaultCurrencyCode: 'EUR',
    productType: { __typename: 'ProductType' as const, id: 'pt', slug: 'top', name: 'Top' }, thumbnail: null, displayImage: null,
  }
  const variant = (id: string, name: string) => ({ __typename: 'Product' as const, id, name, sku: id.toUpperCase(), kind: 'SIMPLE' as const, defaultPrice: 4900, defaultCurrencyCode: 'EUR' })
  const pickHoodie = async (eachVariant: boolean) => {
    const option = { id: 'o1', inputType: 'RADIO' as const, isRequired: true, sortOrder: 0, minSelections: 0, maxSelections: null, title: 'Size', content: [], selections: [] }
    renderSection(<PrefsContext.Provider value=""><BundleStructureSection canManage product={{ id: 'p1', bundleOptions: [option] }} /></PrefsContext.Provider>, [
      { request: { query: ProductPickerPagedDocument, variables: () => true }, result: { data: { productsPaged: { __typename: 'ProductPage' as const, totalCount: 1, items: [hoodie] } } }, maxUsageCount: 10 },
      { request: { query: ProductTypesDocument }, result: { data: { productTypes: [] } }, maxUsageCount: 10 },
      { request: { query: ProductVariantsDocument, variables: { id: 'cfg' } }, result: { data: { product: { __typename: 'Product' as const, id: 'cfg', children: [variant('h-s', 'Hoodie S'), variant('h-m', 'Hoodie M')] } } } },
    ])
    fireEvent.click(await screen.findByRole('button', { name: 'Add selection' }))
    const dialog = await screen.findByRole('dialog')
    fireEvent.click(await within(dialog).findByRole('checkbox', { name: 'Select Hoodie' }, SLOW))
    if (eachVariant) fireEvent.click(within(dialog).getByRole('checkbox', { name: 'Add each variant as its own selection' }))
    fireEvent.click(within(dialog).getByRole('button', { name: /Stage 1 selection/ }))
  }

  it('keeps a configurable as one selection the customer picks a variant of', async () => {
    await pickHoodie(false)
    expect(await screen.findByText('customer picks the variant', undefined, SLOW)).toBeTruthy()
    expect(screen.getByRole('switch', { name: 'Hoodie is a default' })).toHaveProperty('disabled', true)
    expect(screen.queryByLabelText('Hoodie S quantity')).toBeNull()
  })

  it('stages each variant when asked to', async () => {
    await pickHoodie(true)
    expect(await screen.findByLabelText('Hoodie S quantity', undefined, SLOW)).toBeTruthy()
    expect(screen.getByLabelText('Hoodie M quantity')).toBeTruthy()
  })
})

// What this pins: a selection the option's rule made is shown as such and can't be edited, removed or dragged - the rule is
describe('bundle rule selections', () => {
  it('locks the rows a rule made', async () => {
    const option = {
      id: 'o1', inputType: 'CHECKBOX' as const, isRequired: false, sortOrder: 0, minSelections: 0, maxSelections: null, title: 'Cups', content: [],
      rule: { categoryId: 'c1', attributeValueIds: [], quantity: 1 },
      selections: [
        { id: 's1', product: { ...widget, name: 'Mug' }, quantity: 1, isDefault: false, sortOrder: 0 },
        { id: 'r1', product: { ...widget, id: 'p9', name: 'Cup' }, quantity: 1, isDefault: false, sortOrder: 1, fromRule: true },
      ],
    }
    renderSection(<BundleStructureSection canManage product={{ id: 'p1', bundleOptions: [option] }} />)
    expect(await screen.findByText('from the rule')).toBeTruthy()
    expect(screen.getByText(/fills from a category/)).toBeTruthy()
    expect(screen.getAllByRole('button', { name: 'Remove selection' })).toHaveLength(1)
    expect(screen.queryByLabelText('Cup quantity')).toBeNull()
    expect(screen.queryByRole('button', { name: 'Reorder Cup' })).toBeNull()
  })
})

// What this pins: a row's add-on deal and customer-set quantity stage like its other fields, and the deal moves the price line
describe('bundle selection deals', () => {
  it('stages a percent off and a customer quantity', async () => {
    const listings = [{ isPublished: true, channel: { id: 'c1', name: 'EU store', currencyCode: 'EUR' } }]
    const mug = { ...widget, name: 'Mug', listings: [{ channelId: 'c1', isPublished: true, effectivePrice: 1000, inStock: true }] }
    const option = {
      id: 'o1', inputType: 'RADIO' as const, isRequired: true, sortOrder: 0, minSelections: 0, maxSelections: null, title: 'Cup', content: [],
      selections: [{ id: 's1', product: mug, quantity: 1, isDefault: true, sortOrder: 0 }],
    }
    renderSection(<BundleStructureSection canManage product={{ id: 'p1', listings, bundleOptions: [option] }} />)
    fireEvent.change(await screen.findByLabelText('Mug percent off'), { target: { value: '20' } })
    expect((await screen.findByLabelText('Bundle health')).textContent).toMatch(/Customers pay .*8\.00/)
    fireEvent.click(screen.getByRole('switch', { name: 'Customer sets how many Mug' }))
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    const text = (await screen.findByRole('alertdialog')).textContent
    expect(text).toMatch(/Selection · Mug · % off/)
    expect(text).toMatch(/Selection · Mug · customer sets qty/)
  })
})
