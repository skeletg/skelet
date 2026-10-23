import { useState } from 'react'
import { fireEvent, screen, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { useLocation } from 'react-router-dom'
import { SLOW, captureInput, renderPage } from '@/test/page-harness'
import { CurrenciesPickerDocument } from '@/features/currencies/Currencies.gql'
import { CreateOptionTemplateDocument, OptionTemplatesDocument } from '../Catalog.gql'
import { TemplatesDialog } from './TemplatesDialog'

// What these pin: the dialog lists every template with what it reaches, finds one by name or description, a failed
// "Use here" says why and keeps the dialog, Edit opens the template with the way back, and a new template is created,
// used here and opened - in that order, so its page shows it in use

Element.prototype.hasPointerCapture ??= () => false
Element.prototype.releasePointerCapture ??= () => {}
Element.prototype.scrollIntoView ??= () => {}

const summary = (id: string, name: string, over: object = {}) => ({
  __typename: 'OptionTemplate' as const, id, name, description: '', currencyCode: 'USD', optionCount: 2, ruleCount: 1,
  productCount: 0, productTypeCount: 0, updatedAt: '2026-10-01T00:00:00Z', ...over,
})
const list = [
  summary('t1', 'Gift wrapping', { description: 'Paper and ribbon', productCount: 3, productTypeCount: 1 }),
  summary('t2', 'Engraving'),
]

function Opener(props: Omit<Parameters<typeof TemplatesDialog>[0], 'open' | 'onOpenChange'>) {
  const [open, setOpen] = useState(true)
  return (
    <>
      <p>{open ? 'dialog open' : 'dialog closed'}</p>
      <TemplatesDialog open={open} onOpenChange={setOpen} {...props} />
    </>
  )
}

function TemplatePage() {
  const location = useLocation()
  return <p>template page {location.pathname} from {(location.state as { from?: string } | null)?.from ?? 'nowhere'}</p>
}

function rig(props: Partial<Parameters<typeof TemplatesDialog>[0]> = {}, extra: Parameters<typeof renderPage>[0]['mocks'] = []) {
  renderPage({
    path: '/products/p1',
    routes: {
      '/products/p1': <Opener usedIds={new Set()} canCreate {...props} />,
      '/catalog/configurator-templates/:id': <TemplatePage />,
    },
    mocks: [
      { request: { query: OptionTemplatesDocument }, result: { data: { optionTemplates: list } }, maxUsageCount: 5 },
      {
        request: { query: CurrenciesPickerDocument },
        result: { data: { currencies: [{ __typename: 'Currency', code: 'EUR', name: 'Euro', symbol: '€', decimals: 2, symbolPosition: 'BEFORE' }] } },
        maxUsageCount: 3,
      },
      ...extra,
    ],
  })
}

const dialog = () => screen.findByRole('dialog', { name: /templates?/i }, SLOW)
const row = (name: string) => within(screen.getByRole('list', { name: 'Templates' })).getByText(name).closest('li') as HTMLElement

describe('TemplatesDialog', () => {
  it('lists each template with what it reaches and finds one by its description', async () => {
    rig()
    await within(await dialog()).findByText('Gift wrapping', undefined, SLOW)
    expect(within(row('Gift wrapping')).getByText('2 options · 1 rule · used by 3 products · 1 product type')).toBeDefined()
    expect(within(row('Engraving')).getByText('2 options · 1 rule · not used yet')).toBeDefined()
    fireEvent.change(screen.getByRole('textbox', { name: 'Search templates' }), { target: { value: 'ribbon' } })
    expect(within(screen.getByRole('list', { name: 'Templates' })).getAllByRole('listitem')).toHaveLength(1)
    fireEvent.change(screen.getByRole('textbox', { name: 'Search templates' }), { target: { value: 'zebra' } })
    expect(screen.getByText('No template matches.')).toBeDefined()
  })

  it('says why a template could not be used and stays open', async () => {
    const onUse = vi.fn().mockRejectedValue(new Error('a product of this type reads it'))
    rig({ onUse })
    await within(await dialog()).findByText('Engraving', undefined, SLOW)
    fireEvent.click(within(row('Engraving')).getByRole('button', { name: 'Use here' }))
    expect(await screen.findByText('a product of this type reads it', undefined, SLOW)).toBeDefined()
    expect(onUse).toHaveBeenCalledWith('t2')
    expect(screen.getByText('dialog open')).toBeDefined()
  })

  it('opens a template to edit with the way back to where it came from', async () => {
    rig()
    await within(await dialog()).findByText('Engraving', undefined, SLOW)
    fireEvent.click(within(row('Engraving')).getByRole('link', { name: 'Edit Engraving' }))
    expect(await screen.findByText('template page /catalog/configurator-templates/t2 from /products/p1', undefined, SLOW)).toBeDefined()
  })

  it('creates a template, uses it here and opens it', async () => {
    const created = vi.fn()
    const order: string[] = []
    const onUse = vi.fn(async (id: string) => {
      order.push(`use ${id}`)
    })
    rig({ onUse, defaultCurrency: 'EUR' }, [
      {
        request: { query: CreateOptionTemplateDocument, variables: captureInput((v) => (created(v), order.push('create'))) },
        result: { data: { createOptionTemplate: summary('t9', 'Monogram', { currencyCode: 'EUR' }) } },
      },
    ])
    fireEvent.click(await within(await dialog()).findByRole('button', { name: 'New template' }, SLOW))
    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'Monogram' } })
    fireEvent.click(screen.getByRole('button', { name: 'Create and use here' }))
    expect(await screen.findByText('template page /catalog/configurator-templates/t9 from /products/p1', undefined, SLOW)).toBeDefined()
    expect(created).toHaveBeenCalledWith({ name: 'Monogram', description: '', currencyCode: 'EUR' })
    expect(order).toEqual(['create', 'use t9'])
  })

  it('asks for a name before creating anything', async () => {
    rig()
    fireEvent.click(await within(await dialog()).findByRole('button', { name: 'New template' }, SLOW))
    fireEvent.click(screen.getByRole('button', { name: 'Create template' }))
    expect(screen.getByText('Give it a name.')).toBeDefined()
  })

  it('offers no new template or Use here when it only lists', async () => {
    rig({ canCreate: false, usedIds: new Set(['t1']) })
    await within(await dialog()).findByText('Engraving', undefined, SLOW)
    expect(screen.queryByRole('button', { name: 'New template' })).toBeNull()
    expect(screen.queryByRole('button', { name: 'Use here' })).toBeNull()
    expect(within(row('Gift wrapping')).getByText('In use here')).toBeDefined()
  })
})
