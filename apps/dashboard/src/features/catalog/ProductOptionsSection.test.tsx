import { fireEvent, screen, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, captureInput, renderSection } from '@/test/page-harness'
import {
  AddProductOptionDocument,
  ProductPickerDocument,
  SetProductOptionChannelPriceDocument,
  SetProductOptionValueChannelPriceDocument,
  UpdateProductOptionValueDocument,
} from './Catalog.gql'
import { ProductOptionsSection } from './ProductOptionsSection'

// Channel amounts stage into the Save dialog by channel name, and saving writes only the channels that moved

const product = {
  id: 'p1',
  contentLocale: 'en',
  defaultCurrencyCode: 'USD',
  listings: [
    { channel: { id: 'c-usd', name: 'Wheels', currencyCode: 'USD' } },
    { channel: { id: 'c-eur', name: 'Tyres', currencyCode: 'EUR' } },
  ],
  productOptions: [
    {
      id: 'o1',
      inputType: 'DROPDOWN' as const,
      isRequired: false,
      sortOrder: 0,
      title: 'Finish',
      helpText: '',
      content: [{ locale: 'en', title: 'Finish', helpText: '' }],
      priceValue: null,
      priceType: null,
      channelPrices: [],
      values: [
        {
          id: 'v1',
          sortOrder: 0,
          priceValue: 500,
          priceType: 'FIXED' as const,
          isDefault: false,
          quantity: 1,
          title: 'Gloss',
          content: [{ locale: 'en', title: 'Gloss' }],
          channelPrices: [{ channelId: 'c-eur', priceValue: 400 }],
        },
      ],
    },
  ],
}

const addedOption = {
  __typename: 'ProductOption' as const,
  id: 'o2',
  inputType: 'FIELD' as const,
  isRequired: false,
  sortOrder: 1,
  title: 'Engraving',
  helpText: '',
  content: [{ __typename: 'ProductOptionContent' as const, locale: 'en', title: 'Engraving', helpText: '' }],
  priceValue: 300,
  priceType: 'FIXED' as const,
  maxChars: null,
  maxFileBytes: null,
  fileExtensions: null,
  visibleIf: null,
  channelPrices: [],
  values: [],
  createdAt: '2026-10-01T00:00:00Z',
  updatedAt: '2026-10-01T00:00:00Z',
}

function renderOptions() {
  const writes = { price: vi.fn(), update: vi.fn(), add: vi.fn(), optionPrice: vi.fn() }
  const reply = { __typename: 'Product' as const, id: 'p1', productOptions: [] }
  renderSection(<ProductOptionsSection product={product} canManage />, [
    {
      request: { query: ProductPickerDocument, variables: { search: null, kinds: ['SIMPLE'], limit: 12 } },
      result: { data: { productsPaged: { __typename: 'ProductPage', items: [] } } },
      maxUsageCount: 4,
    },
    {
      request: { query: SetProductOptionValueChannelPriceDocument, variables: captureInput(writes.price) },
      result: { data: { setProductOptionValueChannelPrice: reply } },
      maxUsageCount: 4,
    },
    {
      request: { query: UpdateProductOptionValueDocument, variables: captureInput(writes.update) },
      result: { data: { updateProductOptionValue: reply } },
      maxUsageCount: 2,
    },
    {
      request: { query: AddProductOptionDocument, variables: captureInput(writes.add) },
      result: { data: { addProductOption: { ...reply, productOptions: [addedOption] } } },
    },
    {
      request: { query: SetProductOptionChannelPriceDocument, variables: captureInput(writes.optionPrice) },
      result: { data: { setProductOptionChannelPrice: reply } },
      maxUsageCount: 2,
    },
  ])
  return writes
}

describe('ProductOptionsSection per-channel surcharges', () => {
  it('stages channel amounts into the Save dialog and writes only those', async () => {
    const writes = renderOptions()
    fireEvent.click(await screen.findByRole('button', { name: 'Edit value' }, SLOW))
    const tyres = screen.getByLabelText(/Tyres/)
    expect((tyres as HTMLInputElement).value).toBe('400')
    expect(screen.getByLabelText(/Wheels/)).toBeDefined()

    fireEvent.change(tyres, { target: { value: '450' } })
    fireEvent.change(screen.getByLabelText(/Wheels/), { target: { value: '550' } })
    fireEvent.click(screen.getByRole('button', { name: 'Stage edit' }))

    fireEvent.click(await screen.findByRole('button', { name: 'Save changes' }))
    const dialog = await screen.findByRole('alertdialog')
    expect(within(dialog).getByText(/Value · Gloss · price on Tyres \(EUR\)/)).toBeDefined()
    expect(within(dialog).getByText(/Value · Gloss · price on Wheels \(USD\)/)).toBeDefined()
    expect(within(dialog).getByText('400')).toBeDefined()
    expect(within(dialog).getByText('450')).toBeDefined()
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply/ }))

    await vi.waitFor(() => expect(writes.price).toHaveBeenCalledTimes(2), SLOW)
    expect(writes.price).toHaveBeenCalledWith({ valueId: 'v1', channelId: 'c-eur', priceValue: 450 })
    expect(writes.price).toHaveBeenCalledWith({ valueId: 'v1', channelId: 'c-usd', priceValue: 550 })
    expect(writes.update).not.toHaveBeenCalled()
  })

  it('warns when another currency is left without an amount, and clearing one stages a clear', async () => {
    const writes = renderOptions()
    fireEvent.click(await screen.findByRole('button', { name: 'Edit value' }, SLOW))
    expect(screen.queryByText('Not sold on Tyres until set.')).toBeNull()
    fireEvent.change(screen.getByLabelText(/Tyres/), { target: { value: '' } })
    expect(screen.getByText('Not sold on Tyres until set.')).toBeDefined()
    fireEvent.click(screen.getByRole('button', { name: 'Stage edit' }))

    fireEvent.click(await screen.findByRole('button', { name: 'Save changes' }))
    const dialog = await screen.findByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply/ }))
    await vi.waitFor(() => expect(writes.price).toHaveBeenCalledWith({ valueId: 'v1', channelId: 'c-eur', priceValue: null }), SLOW)
    expect(writes.price).toHaveBeenCalledTimes(1)
  })

  it('a new fixed-price option stages its channel amounts and writes them once it exists', async () => {
    const writes = renderOptions()
    fireEvent.click(await screen.findByRole('button', { name: /Add option/ }, SLOW))
    fireEvent.change(screen.getByLabelText('Title'), { target: { value: 'Engraving' } })
    expect(screen.queryByLabelText(/Tyres/)).toBeNull()
    fireEvent.change(screen.getByLabelText('Price adjustment'), { target: { value: '300' } })
    expect(screen.getByText('Not sold on Tyres until set.')).toBeDefined()
    fireEvent.change(screen.getByLabelText(/Tyres/), { target: { value: '280' } })
    fireEvent.click(screen.getByRole('button', { name: 'Stage add' }))

    fireEvent.click(await screen.findByRole('button', { name: 'Save changes' }))
    const dialog = await screen.findByRole('alertdialog')
    expect(within(dialog).getByText(/Tyres/)).toBeDefined()
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply/ }))

    await vi.waitFor(() => expect(writes.optionPrice).toHaveBeenCalledTimes(1), SLOW)
    expect(writes.add).toHaveBeenCalledTimes(1)
    expect(writes.optionPrice).toHaveBeenCalledWith({ optionId: 'o2', channelId: 'c-eur', priceValue: 280 })
  })
})
