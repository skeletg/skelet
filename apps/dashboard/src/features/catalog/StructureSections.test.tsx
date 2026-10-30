import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, captureInput, renderSection } from '@/test/page-harness'
import { SetBundlePricingDocument } from './Catalog.gql'
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
    renderSection(<KitStructureSection canManage product={{ id: 'p1', kitComponents: [{ id: 'kc1', component: widget, quantity: 1, sortOrder: 0 }] }} />)
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
