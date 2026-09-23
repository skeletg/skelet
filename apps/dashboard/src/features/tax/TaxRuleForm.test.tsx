import { fireEvent, render, screen, within } from '@testing-library/react'
import { MockedProvider } from '@apollo/client/testing/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import { TaxRuleEditPage } from './TaxRuleForm'
import { TaxRuleDocument, TaxRuleTypesDocument, UpdateTaxRuleDocument } from './Tax.gql'

// Through the real page: removing a condition lights the header Save (it never did - the section
// registered into a context that wasn't there yet), the dialog names the row, and the update sends it

const stored = {
  __typename: 'TaxRule' as const,
  id: 'r1',
  channelId: 'c1',
  name: 'CA sales tax',
  priority: 0,
  exclusivityGroup: 'jurisdiction',
  sortOrder: 0,
  isActive: true,
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
  conditions: [
    { __typename: 'TaxRuleCondition' as const, type: 'shipping_country', params: { countries: ['US'] }, paramsVersion: 1, sortOrder: 0 },
  ],
  effects: [{ __typename: 'TaxRuleEffect' as const, type: 'rate', params: { rate: 7 }, paramsVersion: 1, sortOrder: 0 }],
}

const types = {
  taxConditionTypes: [
    { __typename: 'TaxRuleTypeDescriptor' as const, slug: 'shipping_country', label: 'Shipping country', description: '', paramsSchema: null },
  ],
  taxEffectTypes: [
    { __typename: 'TaxRuleTypeDescriptor' as const, slug: 'rate', label: 'Rate', description: '', paramsSchema: null },
  ],
}

function renderPage(onWrite = vi.fn()) {
  const mocks = [
    { request: { query: TaxRuleDocument, variables: { id: 'r1' } }, result: { data: { taxRule: stored } } },
    { request: { query: TaxRuleTypesDocument }, result: { data: types } },
    {
      request: {
        query: UpdateTaxRuleDocument,
        variables: (vars: { input: Record<string, unknown> }) => {
          onWrite(vars.input)
          return true
        },
      },
      result: { data: { updateTaxRule: { ...stored, conditions: [] } } },
    },
  ]
  render(
    <MockedProvider mocks={mocks}>
      <MemoryRouter initialEntries={['/tax/r1']}>
        <Routes>
          <Route path="/tax/:id" element={<TaxRuleEditPage />} />
          <Route path="/tax" element={<p>tax list</p>} />
        </Routes>
      </MemoryRouter>
    </MockedProvider>,
  )
  return onWrite
}

describe('TaxRuleEditPage', () => {
  it('reviews and saves a removed condition', async () => {
    const onWrite = renderPage()
    await screen.findByDisplayValue('CA sales tax')
    const save = screen.getByRole('button', { name: 'Save changes' }) as HTMLButtonElement
    expect(save.disabled).toBe(true)

    fireEvent.click(screen.getByRole('button', { name: 'Remove conditions 1' }))
    expect(save.disabled).toBe(false)
    fireEvent.click(save)

    const dialog = await screen.findByRole('alertdialog')
    expect(within(dialog).getByText('Conditions · 1')).toBeDefined()
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply 1 change/ }))

    await screen.findByText('tax list')
    expect(onWrite).toHaveBeenCalledWith(
      expect.objectContaining({ id: 'r1', conditions: [], effects: [{ type: 'rate', params: { rate: 7 }, sortOrder: 0 }] }),
    )
  })
})
