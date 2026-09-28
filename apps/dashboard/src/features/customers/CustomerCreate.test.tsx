import { fireEvent, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, captureInput, globalScope, renderPage } from '@/test/page-harness'
import { CustomerCreate } from './CustomerCreate'
import { AdminCreateCustomerDocument } from './Customers.gql'
import { CustomerGroupsDocument } from './CustomerGroups.gql'

const groups = {
  customerGroups: [
    { __typename: 'CustomerGroup', id: 'g1', slug: 'retail', name: 'Retail', description: '', isDefault: true, memberCount: 0, createdAt: '2026-01-01T00:00:00Z' },
    { __typename: 'CustomerGroup', id: 'g2', slug: 'wholesale', name: 'Wholesale', description: '', isDefault: false, memberCount: 0, createdAt: '2026-01-01T00:00:00Z' },
  ],
}

function renderCreate(onCreate = vi.fn(), channel = true) {
  renderPage({
    path: '/customers/new',
    routes: { '/customers/new': <CustomerCreate />, '/customers/:id': <p>customer page</p> },
    scope: channel ? { ...globalScope, effectiveChannelId: 'c1', isChannel: true, isGlobal: false } : globalScope,
    mocks: [
      { request: { query: CustomerGroupsDocument }, result: { data: groups }, maxUsageCount: 3 },
      {
        request: { query: AdminCreateCustomerDocument, variables: captureInput(onCreate) },
        result: { data: { adminCreateCustomer: { __typename: 'Customer', id: 'cu1' } } },
      },
    ],
  })
  return onCreate
}

function fill() {
  fireEvent.change(screen.getByLabelText('First name'), { target: { value: 'Ada' } })
  fireEvent.change(screen.getByLabelText('Last name'), { target: { value: 'Lovelace' } })
  fireEvent.change(screen.getByLabelText('Email'), { target: { value: 'ada@example.com' } })
  fireEvent.change(screen.getByLabelText('Password'), { target: { value: 'correct horse' } })
}

describe('CustomerCreate', () => {
  it('sends the default group when none was picked, then opens the customer', async () => {
    const onCreate = renderCreate()
    expect(((await screen.findByLabelText(/Retail/)) as HTMLInputElement).checked).toBe(true)
    fill()
    fireEvent.click(screen.getByRole('button', { name: 'Create customer' }))
    expect(await screen.findByText('customer page', {}, SLOW)).toBeDefined()
    expect(onCreate).toHaveBeenCalledWith(
      expect.objectContaining({ email: 'ada@example.com', customerGroupSlug: 'retail', channelId: 'c1', phone: null }),
    )
  })

  it('refuses to create without a channel in scope', async () => {
    const onCreate = renderCreate(vi.fn(), false)
    fill()
    fireEvent.click(screen.getByRole('button', { name: 'Create customer' }))
    expect(await screen.findByText(/Pick a channel in the header/, { selector: '[data-slot="alert-description"], div' })).toBeDefined()
    expect(onCreate).not.toHaveBeenCalled()
  })
})
