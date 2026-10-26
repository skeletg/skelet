import { fireEvent, screen, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import { SLOW, captureInput, globalScope, renderSection } from '@/test/page-harness'
import {
  AddProductOptionDocument,
  AddProductOptionValueDocument,
  DeleteProductOptionDocument,
  SetProductOptionChannelPriceDocument,
  SetProductOptionRulesDocument,
  SetProductOptionValueChannelPriceDocument,
  UpdateProductOptionDocument,
  UpdateProductOptionValueDocument,
  UpsertProductOptionContentDocument,
  OptionTemplatesDocument,
  SetProductConfiguratorLayoutDocument,
  SetProductOptionTemplatesDocument,
} from './Catalog.gql'
import { ConfiguratorSection, ProductOptionsSection } from './ProductOptionsSection'

// What these pin: the sheet stages one option with its choices, prices are typed in money not cents, and the save
// writes in an order the backend accepts - conditions and deletes included

// Radix selects reach for these, and jsdom has none
Element.prototype.hasPointerCapture ??= () => false
Element.prototype.releasePointerCapture ??= () => {}
Element.prototype.scrollIntoView ??= () => {}

const T = '2026-10-01T00:00:00Z'

const value = (id: string, title: string, priceValue: number, sortOrder: number, extra: object = {}) => ({
  __typename: 'ProductOptionValue' as const,
  id, sortOrder, priceValue, priceType: 'FIXED' as const, isDefault: false, forcedLineQty: null, quantity: 1, title,
  content: [{ __typename: 'ProductOptionValueContent' as const, locale: 'en', title }],
  sourceProduct: null, channelPrices: [] as { channelId: string; priceValue: number }[], createdAt: T, updatedAt: T,
  ...extra,
})

const option = (id: string, title: string, inputType: 'DROPDOWN' | 'FIELD' | 'AREA', sortOrder: number, extra: object = {}) => ({
  __typename: 'ProductOption' as const,
  id, inputType, isRequired: false, sortOrder, title, helpText: '',
  content: [{ __typename: 'ProductOptionContent' as const, locale: 'en', title, helpText: '' }],
  priceValue: null, priceType: null, maxChars: null, maxFileBytes: null, fileExtensions: null, visibleIf: null as unknown,
  channelPrices: [] as { channelId: string; priceValue: number }[], values: [] as ReturnType<typeof value>[], createdAt: T, updatedAt: T,
  ...extra,
})

const product = {
  id: 'p1',
  contentLocale: 'en',
  defaultCurrencyCode: 'USD',
  listings: [
    { channel: { id: 'c-usd', name: 'Wheels', currencyCode: 'USD' } },
    { channel: { id: 'c-eur', name: 'Tyres', currencyCode: 'EUR' } },
  ],
  productOptions: [
    option('o1', 'Finish', 'DROPDOWN', 0, {
      values: [value('v1', 'Gloss', 500, 0, { channelPrices: [{ channelId: 'c-eur', priceValue: 400 }] }), value('v2', 'Matte', 0, 1)],
    }),
    option('o3', 'Engraving colour', 'FIELD', 1, { visibleIf: { rules: [{ option_id: 'o1', op: 'anyOf', value_ids: ['v1'] }] } }),
    option('o4', 'Gift note', 'AREA', 2),
  ],
}

function renderOptions(scope = globalScope, subject: typeof product & { optionRules?: object[] } = product) {
  const writes = {
    price: vi.fn(), update: vi.fn(), add: vi.fn(), addValue: vi.fn(), optionPrice: vi.fn(), optionUpdate: vi.fn(), del: vi.fn(), content: vi.fn(), rules: vi.fn(),
  }
  const reply = (productOptions: object[] = []) => ({ __typename: 'Product' as const, id: 'p1', productOptions })
  const created = option('o2', 'New', 'DROPDOWN', 3)
  let valuesAdded = 0
  renderSection(<MemoryRouter><ProductOptionsSection product={subject as typeof product} canManage /></MemoryRouter>, [
    { request: { query: SetProductOptionValueChannelPriceDocument, variables: captureInput(writes.price) }, result: { data: { setProductOptionValueChannelPrice: reply() } }, maxUsageCount: 4 },
    { request: { query: UpdateProductOptionValueDocument, variables: captureInput(writes.update) }, result: { data: { updateProductOptionValue: reply() } }, maxUsageCount: 4 },
    { request: { query: AddProductOptionDocument, variables: captureInput(writes.add) }, result: { data: { addProductOption: reply([created]) } } },
    {
      request: { query: AddProductOptionValueDocument, variables: captureInput(writes.addValue) },
      result: () => {
        valuesAdded++
        const values = Array.from({ length: valuesAdded }, (_, i) => value(`v${10 + i}`, `Choice ${i}`, 0, i))
        return { data: { addProductOptionValue: reply([{ ...created, values }]) } }
      },
      maxUsageCount: 4,
    },
    { request: { query: SetProductOptionChannelPriceDocument, variables: captureInput(writes.optionPrice) }, result: { data: { setProductOptionChannelPrice: reply() } }, maxUsageCount: 2 },
    { request: { query: UpdateProductOptionDocument, variables: captureInput(writes.optionUpdate) }, result: { data: { updateProductOption: reply() } }, maxUsageCount: 8 },
    { request: { query: DeleteProductOptionDocument, variables: captureInput(writes.del) }, result: { data: { deleteProductOption: reply() } }, maxUsageCount: 2 },
    { request: { query: UpsertProductOptionContentDocument, variables: captureInput(writes.content) }, result: { data: { upsertProductOptionContent: reply() } }, maxUsageCount: 2 },
    { request: { query: SetProductOptionRulesDocument, variables: captureInput(writes.rules) }, result: { data: { setProductOptionRules: { __typename: 'Product' as const, id: 'p1', optionRules: [] } } } },
  ], scope)
  return writes
}

const sheet = () => screen.getByRole('dialog')

async function saveAndConfirm() {
  fireEvent.click(await screen.findByRole('button', { name: 'Save changes' }))
  const dialog = await screen.findByRole('alertdialog')
  return {
    dialog,
    apply: () => fireEvent.click(within(dialog).getByRole('button', { name: /Apply/ })),
  }
}

describe('ProductOptionsSection', () => {
  it('reads a choice store amount in money and writes only the stores that moved', async () => {
    const writes = renderOptions()
    fireEvent.click(await screen.findByRole('button', { name: 'Edit Finish' }, SLOW))
    fireEvent.click(within(sheet()).getByRole('button', { name: 'More settings for Gloss' }))
    const tyres = within(sheet()).getByLabelText(/Tyres/) as HTMLInputElement
    expect(tyres.value).toBe('4.00')
    fireEvent.change(tyres, { target: { value: '4.50' } })
    fireEvent.change(within(sheet()).getByLabelText(/Wheels/), { target: { value: '5.50' } })
    fireEvent.click(within(sheet()).getByRole('button', { name: 'Done' }))

    const { dialog, apply } = await saveAndConfirm()
    expect(within(dialog).getByText(/Value · Gloss · price on Tyres \(EUR\)/)).toBeDefined()
    expect(within(dialog).getByText(/Value · Gloss · price on Wheels \(USD\)/)).toBeDefined()
    apply()
    await vi.waitFor(() => expect(writes.price).toHaveBeenCalledTimes(2), SLOW)
    expect(writes.price).toHaveBeenCalledWith({ valueId: 'v1', channelId: 'c-eur', priceValue: 450 })
    expect(writes.price).toHaveBeenCalledWith({ valueId: 'v1', channelId: 'c-usd', priceValue: 550 })
    expect(writes.update).not.toHaveBeenCalled()
  })

  it('warns when a store in another currency has no amount, and an emptied one stages a clear', async () => {
    const writes = renderOptions()
    fireEvent.click(await screen.findByRole('button', { name: 'Edit Finish' }, SLOW))
    fireEvent.click(within(sheet()).getByRole('button', { name: 'More settings for Gloss' }))
    expect(within(sheet()).queryByText('Not sold on Tyres until set.')).toBeNull()
    fireEvent.change(within(sheet()).getByLabelText(/Tyres/), { target: { value: '' } })
    expect(within(sheet()).getByText('Not sold on Tyres until set.')).toBeDefined()
    fireEvent.click(within(sheet()).getByRole('button', { name: 'Done' }))

    const { apply } = await saveAndConfirm()
    apply()
    await vi.waitFor(() => expect(writes.price).toHaveBeenCalledWith({ valueId: 'v1', channelId: 'c-eur', priceValue: null }), SLOW)
    expect(writes.price).toHaveBeenCalledTimes(1)
  })

  it('creates an option with its choices in one go, prices typed as money', async () => {
    const writes = renderOptions()
    fireEvent.click(await screen.findByRole('button', { name: 'Add option' }, SLOW))
    const s = sheet()
    fireEvent.change(within(s).getByLabelText(/Name shown to customers/), { target: { value: 'Wrapping' } })
    fireEvent.change(within(s).getByLabelText('Choice 1 name'), { target: { value: 'None' } })
    fireEvent.change(within(s).getByLabelText('Choice 2 name'), { target: { value: 'Gift box' } })
    fireEvent.change(within(s).getByLabelText('Gift box extra charge'), { target: { value: '3.5' } })
    fireEvent.click(within(s).getByRole('button', { name: 'Add a choice' }))
    fireEvent.click(within(s).getByRole('button', { name: 'Add option' }))

    expect(await screen.findByText('Gift box', {}, SLOW)).toBeDefined()
    expect(screen.getByText('+$3.50')).toBeDefined()
    const { dialog, apply } = await saveAndConfirm()
    expect(within(dialog).getByText(/Option · Wrapping/)).toBeDefined()
    apply()
    await vi.waitFor(() => expect(writes.addValue).toHaveBeenCalledTimes(2), SLOW)
    expect(writes.add).toHaveBeenCalledWith(expect.objectContaining({ title: 'Wrapping', inputType: 'DROPDOWN', locale: 'en' }))
    expect(writes.addValue).toHaveBeenNthCalledWith(1, expect.objectContaining({ optionId: 'o2', title: 'None', priceValue: 0, sortOrder: 0 }))
    expect(writes.addValue).toHaveBeenNthCalledWith(2, expect.objectContaining({ optionId: 'o2', title: 'Gift box', priceValue: 350, sortOrder: 1 }))
  })

  it('a new fixed-charge text option stages its store amounts and writes them once it exists', async () => {
    const writes = renderOptions()
    fireEvent.click(await screen.findByRole('button', { name: 'Add option' }, SLOW))
    const s = sheet()
    fireEvent.change(within(s).getByLabelText(/Name shown to customers/), { target: { value: 'Engraving' } })
    fireEvent.click(within(s).getByRole('radio', { name: /Short text/ }))
    expect(within(s).queryByLabelText(/Tyres/)).toBeNull()
    fireEvent.change(within(s).getByLabelText('Extra charge'), { target: { value: '3' } })
    expect(within(s).getByText('Not sold on Tyres until set.')).toBeDefined()
    fireEvent.change(within(s).getByLabelText(/Tyres/), { target: { value: '2.80' } })
    fireEvent.click(within(s).getByRole('button', { name: 'Add option' }))

    const { apply } = await saveAndConfirm()
    apply()
    await vi.waitFor(() => expect(writes.optionPrice).toHaveBeenCalledTimes(1), SLOW)
    expect(writes.add).toHaveBeenCalledWith(expect.objectContaining({ inputType: 'FIELD', priceValue: 300, priceType: 'FIXED' }))
    expect(writes.optionPrice).toHaveBeenCalledWith({ optionId: 'o2', channelId: 'c-eur', priceValue: 280 })
  })

  it('refuses a move that would put a condition above what it reads, and saves a move last-first', async () => {
    const writes = renderOptions()
    expect(await screen.findByRole('button', { name: 'Move Engraving colour up' }, SLOW)).toHaveProperty('disabled', true)
    expect(screen.getByText('Only when Finish is Gloss')).toBeDefined()
    fireEvent.click(screen.getByRole('button', { name: 'Move Gift note up' }))

    const { dialog, apply } = await saveAndConfirm()
    expect(within(dialog).getByText(/Option · Gift note · position/)).toBeDefined()
    expect(within(dialog).getByText(/Option · Engraving colour · position/)).toBeDefined()
    expect(within(dialog).queryByText(/Option · Finish · position/)).toBeNull()
    apply()
    await vi.waitFor(() => expect(writes.optionUpdate).toHaveBeenCalledTimes(3), SLOW)
    expect(writes.optionUpdate.mock.calls.map(([input]) => input)).toEqual([
      { id: 'o3', sortOrder: 5 },
      { id: 'o4', sortOrder: 4 },
      { id: 'o1', sortOrder: 3 },
    ])
  })

  it('keeps a condition and the choice it names from being deleted, and undoes a staged delete', async () => {
    const writes = renderOptions()
    fireEvent.pointerDown(await screen.findByRole('button', { name: 'More actions for Finish' }, SLOW), { button: 0, ctrlKey: false, pointerType: 'mouse' })
    expect((await screen.findByRole('menuitem', { name: /Delete option/ })).getAttribute('aria-disabled')).toBe('true')
    expect(screen.getByText('Used by the condition on Engraving colour')).toBeDefined()
    fireEvent.keyDown(screen.getByRole('menu'), { key: 'Escape' })

    fireEvent.click(screen.getByRole('button', { name: 'Edit Finish' }))
    expect(within(sheet()).getByRole('button', { name: 'Remove Gloss' })).toHaveProperty('disabled', true)
    expect(within(sheet()).getByRole('button', { name: 'Remove Matte' })).toHaveProperty('disabled', false)
    fireEvent.click(within(sheet()).getByRole('button', { name: 'Cancel' }))

    fireEvent.pointerDown(screen.getByRole('button', { name: 'More actions for Gift note' }), { button: 0, ctrlKey: false, pointerType: 'mouse' })
    fireEvent.click(await screen.findByRole('menuitem', { name: /Delete option/ }))
    expect(await screen.findByText('Will remove', {}, SLOW)).toBeDefined()
    fireEvent.click(screen.getByRole('button', { name: 'Undo' }))
    expect(screen.queryByText('Will remove')).toBeNull()
    expect(screen.getByRole('button', { name: 'Save changes' })).toHaveProperty('disabled', true)
    expect(writes.del).not.toHaveBeenCalled()
  })

  it('keeps one choice selected first when customers can pick only one', async () => {
    renderOptions()
    fireEvent.click(await screen.findByRole('button', { name: 'Edit Finish' }, SLOW))
    const gloss = within(sheet()).getByRole('checkbox', { name: 'Pre-select Gloss' })
    const matte = within(sheet()).getByRole('checkbox', { name: 'Pre-select Matte' })
    fireEvent.click(gloss)
    fireEvent.click(matte)
    expect(gloss.getAttribute('aria-checked')).toBe('false')
    expect(matte.getAttribute('aria-checked')).toBe('true')
  })

  it('translating changes only names, written to that language', async () => {
    const writes = renderOptions({ ...globalScope, availableLocales: ['en', 'sr'], effectiveLocale: 'sr' })
    fireEvent.click(await screen.findByRole('button', { name: 'Edit Gift note' }, SLOW))
    const s = sheet()
    expect(within(s).getByText(/Translating to sr/)).toBeDefined()
    expect(within(s).getByLabelText('Required')).toHaveProperty('disabled', true)
    expect(within(s).getByLabelText('Extra charge')).toHaveProperty('disabled', true)
    fireEvent.change(within(s).getByLabelText(/Name shown to customers/), { target: { value: 'Poruka' } })
    fireEvent.click(within(s).getByRole('button', { name: 'Done' }))

    const { apply } = await saveAndConfirm()
    apply()
    await vi.waitFor(() => expect(writes.content).toHaveBeenCalledWith({ optionId: 'o4', locale: 'sr', title: 'Poruka', helpText: '' }), SLOW)
    expect(writes.optionUpdate).not.toHaveBeenCalled()
  })

  it('blocks Done until the option has a name and its choices are complete', async () => {
    renderOptions()
    fireEvent.click(await screen.findByRole('button', { name: 'Add option' }, SLOW))
    const s = sheet()
    fireEvent.change(within(s).getByLabelText('Choice 1 extra charge'), { target: { value: '1.234' } })
    fireEvent.click(within(s).getByRole('button', { name: 'Add option' }))
    expect(within(s).getByText('Give the option a name customers will see.')).toBeDefined()
    expect(within(s).getByText('Name this choice.')).toBeDefined()
    expect(within(s).getByText('Enter an amount in USD, like 2.50.')).toBeDefined()
    expect(screen.getByRole('dialog')).toBe(s)
  })

  describe('rules', () => {
    const withRule = {
      ...product,
      optionRules: [{
        __typename: 'ProductOptionRule' as const,
        id: 'r1',
        label: 'Matte set',
        labels: [{ __typename: 'ProductOptionRuleLabel' as const, locale: 'en', label: 'Matte set' }],
        condition: { rules: [{ option_id: 'o1', op: 'anyOf', value_ids: ['v2'] }] },
        actions: [{ __typename: 'ProductOptionRuleAction' as const, type: 'REQUIRE' as const, optionId: 'o4', quantity: null, priceType: null, priceValue: null, channelPrices: [] }],
      }],
    }
    const pick = async (combobox: string, option: string) => {
      const trigger = within(sheet()).getByRole('combobox', { name: combobox })
      fireEvent.pointerDown(trigger, { button: 0, ctrlKey: false, pointerType: 'mouse' })
      fireEvent.click(await screen.findByRole('option', { name: option }))
    }

    it('reads a rule in words and keeps what it names from being deleted', async () => {
      renderOptions(globalScope, withRule)
      expect(await screen.findByText('When Finish is Matte', {}, SLOW)).toBeDefined()
      expect(screen.getByText('Require Gift note')).toBeDefined()
      fireEvent.pointerDown(screen.getByRole('button', { name: 'More actions for Gift note' }), { button: 0, ctrlKey: false, pointerType: 'mouse' })
      expect((await screen.findByRole('menuitem', { name: /Delete option/ })).getAttribute('aria-disabled')).toBe('true')
      expect(screen.getByText('Used by the rule "Matte set"')).toBeDefined()
      fireEvent.keyDown(screen.getByRole('menu'), { key: 'Escape' })
      fireEvent.click(screen.getByRole('button', { name: 'Edit Finish' }))
      expect(within(sheet()).getByRole('button', { name: 'Remove Matte' })).toHaveProperty('disabled', true)
    })

    it('adds a rule in its dialog and saves the whole list in one write', async () => {
      const writes = renderOptions(globalScope, withRule)
      fireEvent.click(await screen.findByRole('button', { name: 'Add rule' }, SLOW))
      fireEvent.click(within(sheet()).getByRole('button', { name: 'Add a condition' }))
      await pick('Condition 1 option', 'Finish')
      fireEvent.click(within(sheet()).getByRole('checkbox', { name: 'Gloss' }))
      await pick('Action 1 option', 'Gift note')
      fireEvent.click(within(sheet()).getByRole('button', { name: 'Add an action' }))
      await pick('Action 2 type', 'Change the price')
      fireEvent.change(within(sheet()).getByLabelText('Action 2 amount'), { target: { value: '-2.5' } })
      fireEvent.change(within(sheet()).getByLabelText(/Name on the cart line/), { target: { value: 'Gloss set' } })
      fireEvent.click(within(sheet()).getByRole('button', { name: 'Add rule' }))

      expect(await screen.findByText('When Finish is Gloss', {}, SLOW)).toBeDefined()
      expect(screen.getByText('Price −$2.50')).toBeDefined()
      const { dialog, apply } = await saveAndConfirm()
      expect(within(dialog).getByText('Rule 2')).toBeDefined()
      apply()
      await vi.waitFor(() => expect(writes.rules).toHaveBeenCalledTimes(1), SLOW)
      expect(writes.rules).toHaveBeenCalledWith({
        productId: 'p1',
        rules: [
          { id: 'r1', labels: [{ locale: 'en', label: 'Matte set' }], condition: withRule.optionRules[0].condition, actions: [{ type: 'REQUIRE', optionId: 'o4' }] },
          {
            labels: [{ locale: 'en', label: 'Gloss set' }],
            condition: { rules: [{ option_id: 'o1', op: 'anyOf', value_ids: ['v1'] }] },
            actions: [{ type: 'REQUIRE', optionId: 'o4' }, { type: 'ADJUST_PRICE', priceType: 'FIXED', priceValue: -250, channelPrices: [] }],
          },
        ],
      })
    })

    it('moves a rule and saves the list in its new order', async () => {
      const second = {
        ...withRule.optionRules[0], id: 'r2', label: '', labels: [],
        actions: [{ ...withRule.optionRules[0].actions[0], type: 'SET_QUANTITY' as const, optionId: null, quantity: 2 }],
      }
      const writes = renderOptions(globalScope, { ...withRule, optionRules: [...withRule.optionRules, second] })
      expect((await screen.findByRole('button', { name: 'Move rule 1 up' }, SLOW))).toHaveProperty('disabled', true)
      fireEvent.click(screen.getByRole('button', { name: 'Move rule 2 up' }))
      const { dialog, apply } = await saveAndConfirm()
      expect(within(dialog).getByText('Rule · Matte set · position')).toBeDefined()
      apply()
      await vi.waitFor(() => expect(writes.rules).toHaveBeenCalledTimes(1), SLOW)
      expect(writes.rules.mock.calls[0][0].rules.map((r: { id?: string }) => r.id)).toEqual(['r2', 'r1'])
    })

    it('refuses a rule with nothing to react to', async () => {
      renderOptions(globalScope, withRule)
      fireEvent.click(await screen.findByRole('button', { name: 'Add rule' }, SLOW))
      fireEvent.click(within(sheet()).getByRole('button', { name: 'Add rule' }))
      expect(within(sheet()).getByText('Add a condition - a rule needs an answer to react to.')).toBeDefined()
      expect(within(sheet()).getByText('Pick an option.')).toBeDefined()
    })

    it('draws the options top to bottom with their logic on a map', async () => {
      renderOptions(globalScope, withRule)
      fireEvent.mouseDown(await screen.findByRole('tab', { name: 'Map' }, SLOW), { button: 0, ctrlKey: false })
      const map = await screen.findByTestId('options-map', {}, SLOW)
      expect(await within(map).findByText('Matte set', {}, SLOW)).toBeDefined()
      expect(within(map).getByText('Customer opens the product')).toBeDefined()
      expect(within(map).getByText('Add to cart')).toBeDefined()
      expect(within(map).getByText(/^Only when Finish is/)).toBeDefined()
    })
  })

  describe('templates', () => {
    const fromTemplate = option('t-o1', 'Wrapping', 'DROPDOWN', -1073741824, {
      templateId: 't1', templateName: 'Gift wrapping', values: [value('t-v1', 'Gift', 300, 0)],
    })
    const templates = [{ __typename: 'OptionTemplate' as const, id: 't1', name: 'Gift wrapping', optionCount: 1, ruleCount: 0 }]
    const noTemplates = { request: { query: OptionTemplatesDocument }, result: { data: { optionTemplates: [] } } }

    it("shows a template's options read-only under its name, and keeps it while an own option reads them", async () => {
      const cardNote = option('o5', 'Card note', 'AREA', 3, { visibleIf: { rules: [{ option_id: 't-o1', op: 'anyOf', value_ids: ['t-v1'] }] } })
      const subject = { ...product, productOptions: [fromTemplate, cardNote], optionTemplates: templates }
      renderSection(<MemoryRouter><ProductOptionsSection product={subject} canManage /></MemoryRouter>, [noTemplates])
      const group = await screen.findByRole('region', { name: 'From the template Gift wrapping' }, SLOW)
      expect(within(group).getByText('Wrapping')).toBeDefined()
      expect(within(group).queryByRole('button', { name: 'Edit Wrapping' })).toBeNull()
      expect(within(group).getByRole('link', { name: 'Edit template' }).getAttribute('href')).toBe('/catalog/configurator-templates/t1')
      expect(screen.getByRole('button', { name: 'Edit Card note' })).toBeDefined()
      expect(screen.getByText('Only when Wrapping is Gift')).toBeDefined()

      fireEvent.pointerDown(within(group).getByRole('button', { name: 'Actions for the template Gift wrapping' }), { button: 0, ctrlKey: false, pointerType: 'mouse' })
      expect((await screen.findByRole('menuitem', { name: /Remove from this product/ })).getAttribute('aria-disabled')).toBe('true')
      expect(screen.getByText('Used by the condition on Card note')).toBeDefined()
    })

    const cover = option('ty-o1', 'Cover', 'DROPDOWN', -1073741825, { templateId: 't2', templateName: 'Care plan', values: [value('ty-v1', 'Yearly', 300, 0)] })
    const carePlan = { __typename: 'OptionTemplate' as const, id: 't2', name: 'Care plan', optionCount: 1, ruleCount: 0 }
    const mugs = { id: 'pt1', name: 'Mugs' }
    const summary = (id: string, name: string) => ({
      __typename: 'OptionTemplate' as const, id, name, description: '', currencyCode: 'USD', optionCount: 1, ruleCount: 0, productCount: 0, productTypeCount: 0, updatedAt: T,
    })
    const catalogTemplates = {
      request: { query: OptionTemplatesDocument },
      result: { data: { optionTemplates: [summary('t1', 'Gift wrapping'), summary('t2', 'Care plan'), summary('t3', 'Monogram')] } },
      maxUsageCount: 5,
    }

    it("shows its product type's templates first, read-only, with the way to change them", async () => {
      const subject = { ...product, productOptions: [cover, fromTemplate, product.productOptions[2]], optionTemplates: templates, typeOptionTemplates: [carePlan], productType: mugs }
      renderSection(<MemoryRouter><ProductOptionsSection product={subject} canManage /></MemoryRouter>, [noTemplates])
      const fromType = await screen.findByRole('region', { name: 'From the product type Mugs: Care plan' }, SLOW)
      const picked = screen.getByRole('region', { name: 'From the template Gift wrapping' })
      expect(fromType.compareDocumentPosition(picked) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
      expect(within(fromType).getByText('Cover')).toBeDefined()
      expect(within(fromType).getByRole('link', { name: 'Change on the type' }).getAttribute('href')).toBe('/catalog/product-types/pt1')
      expect(within(fromType).getByRole('link', { name: 'Edit template' }).getAttribute('href')).toBe('/catalog/configurator-templates/t2')
      expect(within(fromType).queryByRole('button', { name: /Actions for the template/ })).toBeNull()
      expect(screen.getByText('3 options, shown to customers in this order.')).toBeDefined()
    })

    it('shows a pick its type already hands over once, lets it go, and won\'t detach it', async () => {
      const reader = option('o6', 'Plan holder', 'FIELD', 3, { visibleIf: { rules: [{ option_id: 'ty-o1', op: 'anyOf', value_ids: ['ty-v1'] }] } })
      const subject = { ...product, productOptions: [cover, reader], optionTemplates: [carePlan], typeOptionTemplates: [carePlan], productType: mugs }
      renderSection(<MemoryRouter><ProductOptionsSection product={subject} canManage /></MemoryRouter>, [noTemplates])
      const picked = await screen.findByRole('region', { name: 'From the template Care plan' }, SLOW)
      expect(within(picked).getByText(/Its product type hands it over too/)).toBeDefined()
      expect(screen.getAllByText('Cover')).toHaveLength(1)

      fireEvent.pointerDown(within(picked).getByRole('button', { name: 'Actions for the template Care plan' }), { button: 0, ctrlKey: false, pointerType: 'mouse' })
      expect((await screen.findByRole('menuitem', { name: /Remove from this product/ })).getAttribute('aria-disabled')).toBeNull()
      expect(screen.getByRole('menuitem', { name: /Detach as own options/ }).getAttribute('aria-disabled')).toBe('true')
      expect(screen.getByText('It comes from the product type too')).toBeDefined()
    })

    it('says the options stay when removing a pick the type hands over too, then sends the rest', async () => {
      const sent = vi.fn()
      const subject = { ...product, productOptions: [cover, fromTemplate], optionTemplates: [templates[0], carePlan], typeOptionTemplates: [carePlan], productType: mugs }
      renderSection(<MemoryRouter><ProductOptionsSection product={subject} canManage /></MemoryRouter>, [
        noTemplates,
        {
          request: { query: SetProductOptionTemplatesDocument, variables: captureInput(sent) },
          result: { data: { setProductOptionTemplates: { __typename: 'Product' as const, id: 'p1', optionTemplates: [], typeOptionTemplates: [], productOptions: [], optionRules: [] } } },
        },
      ])
      const picked = await screen.findByRole('region', { name: 'From the template Care plan' }, SLOW)
      fireEvent.pointerDown(within(picked).getByRole('button', { name: 'Actions for the template Care plan' }), { button: 0, ctrlKey: false, pointerType: 'mouse' })
      fireEvent.click(await screen.findByRole('menuitem', { name: /Remove from this product/ }))
      const confirm = await screen.findByRole('alertdialog', {}, SLOW)
      expect(within(confirm).getByText(/still hands it over, so its options keep showing/)).toBeDefined()
      fireEvent.click(within(confirm).getByRole('button', { name: 'Remove' }))
      await vi.waitFor(() => expect(sent).toHaveBeenCalledWith({ productId: 'p1', templateIds: ['t1'] }), SLOW)
    })

    it('uses a template from the manage dialog, which marks what is in use and what the type hands over', async () => {
      const sent = vi.fn()
      const subject = { ...product, productOptions: [cover, fromTemplate], optionTemplates: templates, typeOptionTemplates: [carePlan], productType: mugs }
      renderSection(<MemoryRouter><ProductOptionsSection product={subject} canManage /></MemoryRouter>, [
        catalogTemplates,
        {
          request: { query: SetProductOptionTemplatesDocument, variables: captureInput(sent) },
          result: { data: { setProductOptionTemplates: { __typename: 'Product' as const, id: 'p1', optionTemplates: [], typeOptionTemplates: [], productOptions: [], optionRules: [] } } },
        },
      ])
      fireEvent.pointerDown(await screen.findByRole('button', { name: 'Use a template' }, SLOW), { button: 0, ctrlKey: false, pointerType: 'mouse' })
      expect(await screen.findByRole('menuitem', { name: /Monogram/ }, SLOW)).toBeDefined()
      expect(screen.queryByRole('menuitem', { name: /Gift wrapping|Care plan/ })).toBeNull()
      fireEvent.click(screen.getByRole('menuitem', { name: 'Manage templates…' }))

      const dialog = await screen.findByRole('dialog', { name: 'Configurator templates' }, SLOW)
      const row = (name: string) => within(dialog).getByText(name).closest('li') as HTMLElement
      expect(within(row('Gift wrapping')).getByText('In use here')).toBeDefined()
      expect(within(row('Care plan')).getByText('From the product type')).toBeDefined()
      expect(within(row('Care plan')).queryByRole('button', { name: 'Use here' })).toBeNull()
      fireEvent.click(within(row('Monogram')).getByRole('button', { name: 'Use here' }))
      await vi.waitFor(() => expect(sent).toHaveBeenCalledWith({ productId: 'p1', templateIds: ['t1', 't3'] }), SLOW)
    })

    it("edits a template's own options on the template, though each names it", async () => {
      renderSection(
        <MemoryRouter>
          <ConfiguratorSection
            subject={{ owner: { kind: 'template', id: 't1' }, contentLocale: 'en', currency: 'USD', channels: [], productOptions: [fromTemplate], optionRules: [] }}
            canManage
          />
        </MemoryRouter>,
      )
      expect(await screen.findByRole('button', { name: 'Edit Wrapping' }, SLOW)).toBeDefined()
      expect(screen.queryByRole('region', { name: /From the template/ })).toBeNull()
      expect(screen.queryByRole('button', { name: 'Use a template' })).toBeNull()
    })
  })

  it('stages the step-by-step layout and saves it with the section', async () => {
    const layout = vi.fn()
    renderSection(
      <MemoryRouter>
        <ProductOptionsSection product={{ ...product, configuratorLayout: 'LIST' as const }} canManage />
      </MemoryRouter>,
      [
        { request: { query: OptionTemplatesDocument }, result: { data: { optionTemplates: [] } } },
        {
          request: { query: SetProductConfiguratorLayoutDocument, variables: captureInput(layout) },
          result: { data: { setProductConfiguratorLayout: { __typename: 'Product' as const, id: 'p1', configuratorLayout: 'STEPS' as const } } },
        },
      ],
    )
    const trigger = await screen.findByRole('combobox', { name: 'Shown as' }, SLOW)
    fireEvent.pointerDown(trigger, { button: 0, ctrlKey: false, pointerType: 'mouse' })
    fireEvent.click(await screen.findByRole('option', { name: 'Step by step' }))
    expect(screen.getByText('One option at a time, with progress and the earlier picks above it.')).toBeDefined()
    const { dialog, apply } = await saveAndConfirm()
    expect(within(dialog).getByText('Shown as')).toBeDefined()
    apply()
    await vi.waitFor(() => expect(layout).toHaveBeenCalledWith({ productId: 'p1', layout: 'STEPS' }), SLOW)
  })
})
