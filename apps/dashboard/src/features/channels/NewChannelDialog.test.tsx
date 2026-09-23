import { fireEvent, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, renderDialog } from '@/test/page-harness'
import { CurrenciesPickerDocument } from '@/features/currencies/Currencies.gql'
import { LocalesPickerDocument } from '@/features/locales/Locales.gql'
import { NewChannelDialog } from './NewChannelDialog'
import { newChannelSchema, toNewChannelInput } from './schemas/newChannel'

// The pickers are Radix selects, which jsdom can't open - the dialog is checked for its field-level
// errors, and the schema for what a filled form sends
describe('new channel schema', () => {
  it('wants a lowercase slug and all four fields', () => {
    const issues = newChannelSchema
      .safeParse({ slug: 'US Store', name: '', currencyCode: '', defaultLocale: '' })
      .error?.issues.map((i) => i.path.join('.'))
    expect(issues).toEqual(['slug', 'name', 'currencyCode', 'defaultLocale'])
  })

  it('uppercases the currency on the way out', () => {
    expect(toNewChannelInput({ slug: ' us ', name: 'US', currencyCode: 'usd', defaultLocale: 'en' })).toEqual({
      slug: 'us',
      name: 'US',
      currencyCode: 'USD',
      defaultLocale: 'en',
    })
  })
})

describe('NewChannelDialog', () => {
  it('shows what is missing instead of one generic error', async () => {
    const onCreated = vi.fn()
    renderDialog(<NewChannelDialog open onOpenChange={() => {}} onCreated={onCreated} />, [
      { request: { query: CurrenciesPickerDocument }, result: { data: { currencies: [] } } },
      { request: { query: LocalesPickerDocument }, result: { data: { locales: [] } } },
    ])
    fireEvent.change(screen.getByLabelText('Slug'), { target: { value: 'US Store' } })
    fireEvent.click(screen.getByRole('button', { name: 'Create channel' }))
    expect(await screen.findByText('Lowercase letters, digits, dashes, underscores only', {}, SLOW)).toBeDefined()
    expect(screen.getByText('Name is required')).toBeDefined()
    expect(screen.getByText('Currency is required')).toBeDefined()
    expect(onCreated).not.toHaveBeenCalled()
  })
})
