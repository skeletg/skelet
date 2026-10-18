import { fireEvent, screen, within } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { SLOW, captureInput, meMock, renderPage } from '@/test/page-harness'
import { CustomerDetail } from './CustomerDetail'
import { CustomerGroupsDocument } from './CustomerGroups.gql'
import {
  AdminSetCustomerConsentDocument,
  AdminUpdateCustomerDocument,
  CustomerConsentHistoryDocument,
  CustomerDocument,
  SetCustomerGroupDocument,
  SetCustomerTaxStatusDocument,
  SetCustomerVatIdDocument,
} from './Customers.gql'

// What these pin: the six former sections are one review, each write goes only when its part
// changed, and a bad email keeps the dialog open instead of reaching the server

const group = (id: string, name: string, isDefault = false) => ({
  __typename: 'CustomerGroup' as const,
  id,
  slug: name.toLowerCase(),
  name,
  description: '',
  isDefault,
  memberCount: 0,
  createdAt: '2026-01-01T00:00:00Z',
})

const customer = {
  __typename: 'Customer' as const,
  id: 'cu1',
  channelId: 'c1',
  email: 'jane@example.com',
  firstName: 'Jane',
  lastName: 'Doe',
  phone: null,
  isActive: true,
  isEmailVerified: true,
  note: '',
  taxExempt: false,
  taxExemptionNote: '',
  vatId: '',
  vatIdStatus: 'unverified',
  vatIdCheckedAt: null,
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
  metadata: [],
  group: { __typename: 'CustomerGroup' as const, id: 'g1', slug: 'retail', name: 'Retail' },
  addresses: [],
  defaultBillingAddress: null,
  defaultShippingAddress: null,
  consents: [
    {
      __typename: 'CustomerConsent' as const,
      kind: 'email_marketing',
      optedIn: false,
      source: 'signup',
      updatedAt: null,
      isDefault: true,
      definition: {
        __typename: 'ConsentDefinition' as const,
        slug: 'email_marketing',
        name: 'Email marketing',
        description: '',
        purpose: 'marketing',
        channel: 'email',
        required: false,
        defaultOptIn: false,
      },
    },
  ],
}

const recorder = (fn: (v: Record<string, unknown>) => void) => (vars: Record<string, unknown>) => {
  fn(vars)
  return true
}

function renderCustomer() {
  const writes = { update: vi.fn(), group: vi.fn(), consent: vi.fn(), tax: vi.fn(), vat: vi.fn() }
  renderPage({
    path: '/customers/cu1',
    adminId: 'admin-1',
    routes: { '/customers/:id': <CustomerDetail /> },
    mocks: [
      meMock(),
      { request: { query: CustomerDocument, variables: { id: 'cu1' } }, result: { data: { customer } }, maxUsageCount: 5 },
      { request: { query: CustomerGroupsDocument }, result: { data: { customerGroups: [group('g1', 'Retail', true), group('g2', 'Wholesale')] } }, maxUsageCount: 3 },
      {
        request: { query: AdminUpdateCustomerDocument, variables: captureInput(writes.update) },
        result: { data: { adminUpdateCustomer: { __typename: 'Customer', id: 'cu1', email: 'jane@example.com', firstName: 'Janet', lastName: 'Doe' } } },
      },
      {
        request: { query: SetCustomerGroupDocument, variables: recorder(writes.group) },
        result: { data: { setCustomerGroup: { __typename: 'Customer', id: 'cu1', group: { __typename: 'CustomerGroup', id: 'g2', slug: 'wholesale', name: 'Wholesale' } } } },
      },
      {
        request: { query: AdminSetCustomerConsentDocument, variables: recorder(writes.consent) },
        result: { data: { adminSetCustomerConsent: { __typename: 'CustomerConsent', kind: 'email_marketing', optedIn: true, source: 'admin' } } },
      },
      { request: { query: SetCustomerTaxStatusDocument, variables: recorder(writes.tax) }, result: { data: { setCustomerTaxStatus: null } } },
      { request: { query: SetCustomerVatIdDocument, variables: recorder(writes.vat) }, result: { data: { setCustomerVatId: null } } },
      {
        request: { query: CustomerConsentHistoryDocument, variables: { customerId: 'cu1' } },
        result: {
          data: {
            customerConsentHistory: [
              { __typename: 'CustomerConsentEvent', id: 'e2', kind: 'email_marketing', optedIn: false, source: 'admin', adminId: 'admin-9', createdAt: '2026-02-02T00:00:00Z' },
              { __typename: 'CustomerConsentEvent', id: 'e1', kind: 'email_marketing', optedIn: true, source: 'signup_form', adminId: null, createdAt: '2026-01-01T00:00:00Z' },
            ],
          },
        },
        maxUsageCount: 5,
      },
    ],
  })
  return writes
}

afterEach(() => localStorage.clear())

describe('CustomerDetail', () => {
  it('reviews profile, group and consent together, and writes only those', async () => {
    const writes = renderCustomer()
    const save = await screen.findByRole('button', { name: 'Save changes' })
    fireEvent.change(screen.getByLabelText('First name'), { target: { value: 'Janet' } })
    fireEvent.click(await screen.findByRole('radio', { name: /Wholesale/ }))
    fireEvent.click(screen.getByRole('switch', { name: 'Toggle Email marketing' }))
    fireEvent.click(save)

    const dialog = await screen.findByRole('alertdialog')
    expect(within(dialog).getByText('Email marketing')).toBeDefined()
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply 3 changes/ }))
    await vi.waitFor(() => expect(writes.consent).toHaveBeenCalled(), SLOW)
    expect(writes.update).toHaveBeenCalledWith(expect.objectContaining({ firstName: 'Janet', lastName: 'Doe', email: 'jane@example.com' }))
    expect(writes.group).toHaveBeenCalledWith({ customerId: 'cu1', groupId: 'g2' })
    expect(writes.consent).toHaveBeenCalledWith({ customerId: 'cu1', kind: 'email_marketing', optedIn: true })
    expect(writes.tax).not.toHaveBeenCalled()
    expect(writes.vat).not.toHaveBeenCalled()
  })

  it('keeps the dialog open on a bad email', async () => {
    const writes = renderCustomer()
    const save = await screen.findByRole('button', { name: 'Save changes' })
    fireEvent.change(screen.getByLabelText('Email'), { target: { value: 'jane@' } })
    fireEvent.click(save)
    const dialog = await screen.findByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply/ }))
    expect(await within(dialog).findByText(/Enter a valid email address/)).toBeDefined()
    expect(writes.update).not.toHaveBeenCalled()
  })

  it('lists the consent history with where each change came from', async () => {
    renderCustomer()
    expect(await screen.findByText('Signup form (confirmed)', undefined, SLOW)).toBeDefined()
    expect(screen.getByText('Opted out')).toBeDefined()
    expect(screen.getByText('Admin')).toBeDefined()
  })
})
